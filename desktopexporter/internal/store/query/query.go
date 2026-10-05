// Package query executes read-only SQL against the viewer store.
package query

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/duckdb/duckdb-go/v2"
)

const DefaultLimit uint64 = 25

var ErrReadOnly = errors.New("query must be one read-only SELECT or supported introspection statement")

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Execute runs one read-only statement and returns the DuckDB-built JSON
// response. The caller owns conn and the enclosing Store.WithDBRead boundary.
func Execute(ctx context.Context, conn *sql.Conn, statement string, limit uint64) (result json.RawMessage, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(statement) == "" {
		return nil, fmt.Errorf("empty SQL: %w", ErrReadOnly)
	}
	originalNames, err := requireReadOnly(ctx, conn, statement)
	if err != nil {
		return nil, err
	}

	if _, err := conn.ExecContext(ctx, "BEGIN TRANSACTION READ ONLY"); err != nil {
		return nil, fmt.Errorf("begin read-only transaction: %w", err)
	}
	transactionOpen := true
	defer func() {
		if transactionOpen {
			err = errors.Join(err, rollback(conn, ctx))
		}
	}()

	sourceNames, databaseTypes, err := describeQuery(ctx, conn, statement)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, errors.Join(ctxErr, err)
		}
		// PrepareContext supports cancellation but, unlike Prepare, accepts a
		// multi-statement string. DuckDB's query table function supplies the
		// existing single-statement check, so keep that failure in ErrReadOnly.
		return nil, fmt.Errorf("%w: %w", err, ErrReadOnly)
	}
	if len(originalNames) != len(databaseTypes) {
		return nil, fmt.Errorf("query metadata changed from %d to %d columns", len(originalNames), len(databaseTypes))
	}
	columns := make([]Column, len(databaseTypes))
	for i := range databaseTypes {
		columns[i] = Column{Name: originalNames[i], Type: databaseTypes[i]}
	}
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, fmt.Errorf("encode query columns: %w", err)
	}

	querySQL, args := responseQuery(sourceNames, databaseTypes, statement, columnsJSON, limit)
	var raw string
	if err := conn.QueryRowContext(ctx, querySQL, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("execute read-only query: %w", err)
	}
	if !json.Valid([]byte(raw)) {
		return nil, errors.New("DuckDB returned invalid query response JSON")
	}
	result = json.RawMessage(raw)

	if err := rollback(conn, ctx); err != nil {
		transactionOpen = false
		return nil, errors.Join(err, discard(conn))
	}
	transactionOpen = false
	return result, nil
}

func describeQuery(ctx context.Context, conn *sql.Conn, statement string) (names, types []string, err error) {
	literal := "'" + strings.ReplaceAll(statement, "'", "''") + "'"
	rows, err := conn.QueryContext(ctx,
		"SELECT column_name, column_type FROM (DESCRIBE SELECT * FROM query("+literal+"))")
	if err != nil {
		return nil, nil, fmt.Errorf("inspect query result: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var name, databaseType string
		if err := rows.Scan(&name, &databaseType); err != nil {
			return nil, nil, fmt.Errorf("scan query metadata: %w", err)
		}
		names = append(names, name)
		types = append(types, databaseType)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read query metadata: %w", err)
	}
	return names, types, nil
}

func responseQuery(names, databaseTypes []string, statement string, columnsJSON []byte, limit uint64) (string, []any) {
	values := make([]string, len(names))
	for i, name := range names {
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		values[i] = "to_json(" + quoted + ")"
		if databaseTypes[i] == "FLOAT" || databaseTypes[i] == "DOUBLE" {
			values[i] = "otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(" + quoted + ")))"
		}
	}
	rowJSON := "json_array(" + strings.Join(values, ", ") + ")"
	limited := "SELECT " + rowJSON + " AS row_json, row_number() OVER () AS row_number FROM query(?)"
	rowsJSON := "coalesce(to_json(list(row_json ORDER BY row_number)), json('[]'))"
	truncated := "false"
	args := []any{string(columnsJSON)}
	if limit < math.MaxUint64 {
		limited += " LIMIT ?"
		rowsJSON = "coalesce(to_json(list(row_json ORDER BY row_number) FILTER (WHERE row_number <= ?)), json('[]'))"
		truncated = "count(*) > ?"
		args = append(args, limit, limit, statement, limit+1)
	} else {
		args = append(args, statement)
	}
	querySQL := "SELECT CAST(json_object(" +
		"'columns', ?::JSON, " +
		"'rows', " + rowsJSON + ", " +
		"'truncated', " + truncated +
		") AS VARCHAR) FROM (" + limited + ") AS limited"
	return querySQL, args
}

func requireReadOnly(ctx context.Context, conn *sql.Conn, statement string) ([]string, error) {
	var names []string
	err := conn.Raw(func(raw any) (returnErr error) {
		duckConn, ok := raw.(*duckdb.Conn)
		if !ok {
			return fmt.Errorf("unexpected database driver %T", raw)
		}
		prepared, err := duckConn.PrepareContext(ctx, statement)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return errors.Join(ctxErr, err)
			}
			return fmt.Errorf("parse one SQL statement: %w: %w", err, ErrReadOnly)
		}
		duckStatement, ok := prepared.(*duckdb.Stmt)
		if !ok {
			_ = prepared.Close()
			return fmt.Errorf("unexpected prepared statement %T", prepared)
		}
		defer func() { returnErr = errors.Join(returnErr, duckStatement.Close()) }()
		statementType, err := duckStatement.StatementType()
		if err != nil {
			return err
		}
		if statementType != duckdb.STATEMENT_TYPE_SELECT {
			return fmt.Errorf("DuckDB statement type %d is not read-only: %w", statementType, ErrReadOnly)
		}
		columnCount, err := duckStatement.ColumnCount()
		if err != nil {
			return err
		}
		names = make([]string, columnCount)
		for i := range columnCount {
			names[i], err = duckStatement.ColumnName(i)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return names, err
}

func rollback(conn *sql.Conn, requestCtx context.Context) error {
	_, err := conn.ExecContext(context.WithoutCancel(requestCtx), "ROLLBACK")
	if err != nil {
		return fmt.Errorf("rollback read-only transaction: %w", err)
	}
	return nil
}

func discard(conn *sql.Conn) error {
	return conn.Raw(func(any) error { return driver.ErrBadConn })
}
