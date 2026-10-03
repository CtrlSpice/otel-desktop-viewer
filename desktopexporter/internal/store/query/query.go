// Package query executes bounded read-only SQL against the viewer-owned store.
package query

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

const (
	DefaultLimit = 25
	MaxLimit     = 1000
	MaxBytes     = 1 << 20
)

var (
	ErrInvalidLimit    = errors.New("query limit must be between 0 and 1000")
	ErrReadOnly        = errors.New("query must be one read-only SELECT or supported introspection statement")
	ErrUnsupportedType = errors.New("query result type is unsupported")
	ErrResultTooLarge  = errors.New("query result exceeds the 1 MiB encoded-result limit")
)

type Column struct {
	Name       string `json:"name"`
	DuckDBType string `json:"duckdbType"`
	Encoding   string `json:"encoding"`
}

type Result struct {
	Columns   []Column `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Limit     int      `json:"limit"`
	RowCount  int      `json:"rowCount"`
	Truncated bool     `json:"truncated"`
}

// Execute validates and runs one statement on conn. The caller owns conn's
// lifetime and the surrounding Store.WithDBRead boundary.
func Execute(ctx context.Context, conn *sql.Conn, statement string, limit int) (result Result, err error) {
	result = Result{Limit: limit, Columns: []Column{}, Rows: [][]any{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if limit < 0 || limit > MaxLimit {
		return result, ErrInvalidLimit
	}
	if strings.TrimSpace(statement) == "" {
		return result, fmt.Errorf("empty SQL: %w", ErrReadOnly)
	}
	originalNames, err := classifyReadOnly(conn, statement)
	if err != nil {
		return result, err
	}

	if _, err := conn.ExecContext(ctx, "BEGIN TRANSACTION READ ONLY"); err != nil {
		return result, fmt.Errorf("begin read-only transaction: %w", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		if rollbackErr := rollback(conn, ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback read-only transaction: %w", rollbackErr), discard(conn))
		}
	}()
	rows, err := conn.QueryContext(ctx, "SELECT * FROM query(?) LIMIT ?", statement, limit+1)
	if err != nil {
		if ctx.Err() != nil {
			return result, fmt.Errorf("execute query: %w", errors.Join(ctx.Err(), err))
		}
		return result, fmt.Errorf("execute supported read-only query: %w: %w", err, ErrReadOnly)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return result, fmt.Errorf("inspect query columns: %w", err)
	}
	if len(columnTypes) != len(originalNames) {
		return result, fmt.Errorf("query metadata changed from %d to %d columns", len(originalNames), len(columnTypes))
	}
	result.Columns = make([]Column, len(columnTypes))
	for i, columnType := range columnTypes {
		column, columnErr := describeColumn(originalNames[i], columnType.DatabaseTypeName())
		if columnErr != nil {
			return result, fmt.Errorf("column %d (%q): %w", i+1, originalNames[i], columnErr)
		}
		result.Columns[i] = column
	}

	encodedBytes, err := json.Marshal(result)
	if err != nil {
		return result, fmt.Errorf("encode query metadata: %w", err)
	}
	budget := len(encodedBytes)
	for rows.Next() {
		values := make([]any, len(result.Columns))
		destinations := make([]any, len(values))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return result, fmt.Errorf("scan query row: %w", err)
		}

		if len(result.Rows) == limit {
			result.Truncated = true
			break
		}
		encodedRow := make([]any, len(values))
		for i, value := range values {
			encodedRow[i], err = encodeValue(result.Columns[i], value)
			if err != nil {
				return result, fmt.Errorf("encode row %d column %d (%q): %w", len(result.Rows)+1, i+1, result.Columns[i].Name, err)
			}
		}
		rowBytes, marshalErr := json.Marshal(encodedRow)
		if marshalErr != nil {
			return result, fmt.Errorf("measure encoded row: %w", marshalErr)
		}
		budget += len(rowBytes) + 1
		if budget > MaxBytes {
			return result, ErrResultTooLarge
		}
		result.Rows = append(result.Rows, encodedRow)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("read query rows: %w", err)
	}
	result.RowCount = len(result.Rows)

	encodedBytes, err = json.Marshal(result)
	if err != nil {
		return result, fmt.Errorf("encode query result: %w", err)
	}
	if len(encodedBytes) > MaxBytes {
		return result, ErrResultTooLarge
	}

	if closeErr := rows.Close(); closeErr != nil {
		return result, closeErr
	}
	if rollbackErr := rollback(conn, ctx); rollbackErr != nil {
		transactionOpen = false
		return result, errors.Join(fmt.Errorf("rollback read-only transaction: %w", rollbackErr), discard(conn))
	}
	transactionOpen = false
	return result, nil
}

func rollback(conn *sql.Conn, requestCtx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), time.Second)
	defer cancel()
	_, err := conn.ExecContext(cleanupCtx, "ROLLBACK")
	return err
}

func discard(conn *sql.Conn) error {
	return conn.Raw(func(any) error { return driver.ErrBadConn })
}

func classifyReadOnly(conn *sql.Conn, statement string) ([]string, error) {
	var names []string
	err := conn.Raw(func(raw any) (returnErr error) {
		duckConn, ok := raw.(*duckdb.Conn)
		if !ok {
			return fmt.Errorf("unexpected database driver %T", raw)
		}
		prepared, err := duckConn.Prepare(statement)
		if err != nil {
			return fmt.Errorf("parse one SQL statement: %w: %w", err, ErrReadOnly)
		}
		duckStmt, ok := prepared.(*duckdb.Stmt)
		if !ok {
			_ = prepared.Close()
			return fmt.Errorf("unexpected prepared statement %T", prepared)
		}
		defer func() { returnErr = errors.Join(returnErr, duckStmt.Close()) }()
		statementType, err := duckStmt.StatementType()
		if err != nil {
			return err
		}
		if statementType != duckdb.STATEMENT_TYPE_SELECT {
			return fmt.Errorf("DuckDB statement type %d is not read-only: %w", statementType, ErrReadOnly)
		}
		columnCount, err := duckStmt.ColumnCount()
		if err != nil {
			return err
		}
		names = make([]string, columnCount)
		for i := range columnCount {
			names[i], err = duckStmt.ColumnName(i)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return names, err
}

func describeColumn(name, databaseType string) (Column, error) {
	typeName := strings.ToUpper(databaseType)
	column := Column{Name: name, DuckDBType: databaseType}
	switch {
	case typeName == "BOOLEAN":
		column.Encoding = "boolean"
	case isIntegerType(typeName):
		column.Encoding = "decimal-string"
	case strings.HasPrefix(typeName, "DECIMAL("):
		column.Encoding = "fixed-decimal-string"
	case typeName == "FLOAT":
		column.Encoding = "float32-number-or-bits"
	case typeName == "DOUBLE":
		column.Encoding = "float64-number-or-bits"
	case typeName == "VARCHAR" || typeName == "ENUM" || typeName == "BIT":
		column.Encoding = "string"
	case typeName == "UUID":
		column.Encoding = "uuid-string"
	case typeName == "BLOB":
		column.Encoding = "bytes-base64"
	case isTemporalType(typeName):
		column.Encoding = "temporal-string"
	default:
		return Column{}, fmt.Errorf("DuckDB type %s; cast JSON to VARCHAR or explicitly project/unnest nested values: %w", databaseType, ErrUnsupportedType)
	}
	return column, nil
}

func isIntegerType(typeName string) bool {
	switch typeName {
	case "TINYINT", "SMALLINT", "INTEGER", "BIGINT", "HUGEINT", "BIGNUM",
		"UTINYINT", "USMALLINT", "UINTEGER", "UBIGINT", "UHUGEINT":
		return true
	default:
		return false
	}
}

func isTemporalType(typeName string) bool {
	return typeName == "DATE" || typeName == "TIME" || typeName == "TIME WITH TIME ZONE" ||
		typeName == "TIMESTAMP" || typeName == "TIMESTAMP_S" || typeName == "TIMESTAMP_MS" ||
		typeName == "TIMESTAMP_NS" || typeName == "TIMESTAMP WITH TIME ZONE"
}

func encodeValue(column Column, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch column.Encoding {
	case "boolean", "string":
		return value, nil
	case "decimal-string":
		return integerString(value)
	case "fixed-decimal-string":
		decimal, ok := value.(duckdb.Decimal)
		if !ok {
			return nil, fmt.Errorf("expected duckdb.Decimal, got %T", value)
		}
		return fixedDecimal(decimal), nil
	case "float32-number-or-bits":
		floating, ok := value.(float32)
		if !ok {
			return nil, fmt.Errorf("expected float32, got %T", value)
		}
		if math.IsNaN(float64(floating)) || math.IsInf(float64(floating), 0) || math.Signbit(float64(floating)) && floating == 0 {
			return fmt.Sprintf("0x%08x", math.Float32bits(floating)), nil
		}
		return floating, nil
	case "float64-number-or-bits":
		floating, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("expected float64, got %T", value)
		}
		if math.IsNaN(floating) || math.IsInf(floating, 0) || math.Signbit(floating) && floating == 0 {
			return fmt.Sprintf("0x%016x", math.Float64bits(floating)), nil
		}
		return floating, nil
	case "uuid-string":
		bytes, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected UUID bytes, got %T", value)
		}
		id, err := uuid.FromBytes(bytes)
		if err != nil {
			return nil, err
		}
		return id.String(), nil
	case "bytes-base64":
		bytes, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected BLOB bytes, got %T", value)
		}
		return base64.StdEncoding.EncodeToString(bytes), nil
	case "temporal-string":
		valueTime, ok := value.(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected time.Time, got %T", value)
		}
		return valueTime.Format(time.RFC3339Nano), nil
	default:
		return nil, fmt.Errorf("unknown encoding %q", column.Encoding)
	}
}

func integerString(value any) (string, error) {
	switch integer := value.(type) {
	case int8:
		return strconv.FormatInt(int64(integer), 10), nil
	case int16:
		return strconv.FormatInt(int64(integer), 10), nil
	case int32:
		return strconv.FormatInt(int64(integer), 10), nil
	case int64:
		return strconv.FormatInt(integer, 10), nil
	case uint8:
		return strconv.FormatUint(uint64(integer), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(integer), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(integer), 10), nil
	case uint64:
		return strconv.FormatUint(integer, 10), nil
	case *big.Int:
		return integer.String(), nil
	default:
		return "", fmt.Errorf("expected integer, got %T", value)
	}
}

func fixedDecimal(decimal duckdb.Decimal) string {
	coefficient := new(big.Int).Set(decimal.Value)
	sign := ""
	if coefficient.Sign() < 0 {
		sign = "-"
		coefficient.Abs(coefficient)
	}
	digits := coefficient.String()
	scale := int(decimal.Scale)
	if scale == 0 {
		return sign + digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	point := len(digits) - scale
	return sign + digits[:point] + "." + digits[point:]
}
