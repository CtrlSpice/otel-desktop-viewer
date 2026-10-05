// Package query executes read-only SQL against the viewer-owned store.
package query

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
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

const DefaultLimit uint64 = 25

var (
	ErrInvalidLimit    = errors.New("query limit must be a non-negative whole number")
	ErrReadOnly        = errors.New("query must be one read-only SELECT or supported introspection statement")
	ErrUnsupportedType = errors.New("query result type has no approved lossless wire representation")
)

type Column struct {
	Name       string `json:"name"`
	DuckDBType string `json:"duckdbType"`
	Encoding   string `json:"encoding"`
}

type MapEntry struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type Result struct {
	Columns   []Column `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Limit     uint64   `json:"limit"`
	RowCount  uint64   `json:"rowCount"`
	Truncated bool     `json:"truncated"`
}

// Execute validates and runs one statement on conn. The caller owns conn's
// lifetime and the surrounding Store.WithDBRead boundary.
func Execute(ctx context.Context, conn *sql.Conn, statement string, limit uint64) (result Result, err error) {
	result = Result{Limit: limit, Columns: []Column{}, Rows: [][]any{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if strings.TrimSpace(statement) == "" {
		return result, fmt.Errorf("empty SQL: %w", ErrReadOnly)
	}
	if limit == ^uint64(0) {
		return result, ErrInvalidLimit
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
		if transactionOpen {
			err = errors.Join(err, rollback(conn, ctx))
		}
	}()

	sourceNames, databaseTypes, err := describeQuery(ctx, conn, statement)
	if err != nil {
		return result, err
	}
	if len(databaseTypes) != len(originalNames) {
		return result, fmt.Errorf("query metadata changed from %d to %d columns", len(originalNames), len(databaseTypes))
	}
	result.Columns = make([]Column, len(databaseTypes))
	for i, databaseType := range databaseTypes {
		result.Columns[i] = describeColumn(originalNames[i], databaseType)
	}

	rows, err := conn.QueryContext(ctx, executionQuery(sourceNames, databaseTypes), statement, limit+1)
	if err != nil {
		return result, fmt.Errorf("execute supported read-only query: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		values := make([]any, len(result.Columns))
		destinations := make([]any, len(values))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return result, fmt.Errorf("scan query row: %w", err)
		}
		if result.RowCount == limit {
			result.Truncated = true
			break
		}

		encodedRow := make([]any, len(values))
		for i, value := range values {
			encodedRow[i], err = encodeValue(result.Columns[i], value)
			if err != nil {
				return result, fmt.Errorf("encode row %d column %d (%q): %w", result.RowCount+1, i+1, result.Columns[i].Name, err)
			}
		}
		result.Rows = append(result.Rows, encodedRow)
		result.RowCount++
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("read query rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	if err := rollback(conn, ctx); err != nil {
		transactionOpen = false
		return result, errors.Join(err, discard(conn))
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

func executionQuery(names, types []string) string {
	projections := make([]string, len(names))
	for i, name := range names {
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		projections[i] = quoted
		if strings.EqualFold(types[i], "JSON") {
			projections[i] = "CAST(" + quoted + " AS VARCHAR) AS " + quoted
		}
	}
	return "SELECT " + strings.Join(projections, ", ") + " FROM query(?) LIMIT ?"
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

func describeColumn(name, databaseType string) Column {
	typeName := strings.ToUpper(databaseType)
	column := Column{Name: name, DuckDBType: databaseType, Encoding: "recursive"}
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
	case typeName == "JSON":
		column.Encoding = "json-text"
	case typeName == "UUID":
		column.Encoding = "uuid-string"
	case typeName == "BLOB":
		column.Encoding = "bytes-base64"
	case isTemporalType(typeName):
		column.Encoding = "temporal-string"
	}
	return column
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
	case "boolean", "string", "json-text":
		return value, nil
	case "decimal-string":
		return integerString(value)
	case "fixed-decimal-string":
		decimal, ok := value.(duckdb.Decimal)
		if !ok {
			return nil, fmt.Errorf("expected duckdb.Decimal, got %T", value)
		}
		return fixedDecimal(decimal), nil
	case "float32-number-or-bits", "float64-number-or-bits":
		return encodeFloat(value)
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
	case "recursive":
		return encodeNested(value)
	default:
		return nil, fmt.Errorf("unknown encoding %q", column.Encoding)
	}
}

func encodeNested(value any) (any, error) {
	switch value := value.(type) {
	case nil, bool, string:
		return value, nil
	case int8, int16, int32, int64, uint8, uint16, uint32, uint64, *big.Int:
		return integerString(value)
	case float32, float64:
		return encodeFloat(value)
	case duckdb.Decimal:
		return fixedDecimal(value), nil
	case []byte:
		return base64.StdEncoding.EncodeToString(value), nil
	case time.Time:
		return value.Format(time.RFC3339Nano), nil
	case []any:
		encoded := make([]any, len(value))
		for i, item := range value {
			var err error
			encoded[i], err = encodeNested(item)
			if err != nil {
				return nil, fmt.Errorf("list item %d: %w", i, err)
			}
		}
		return encoded, nil
	case map[string]any:
		encoded := make(map[string]any, len(value))
		for key, item := range value {
			var err error
			encoded[key], err = encodeNested(item)
			if err != nil {
				return nil, fmt.Errorf("struct field %q: %w", key, err)
			}
		}
		return encoded, nil
	case duckdb.OrderedMap:
		keys, values := value.Keys(), value.Values()
		entries := make([]MapEntry, len(keys))
		for i, key := range keys {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("DuckDB MAP key %T: %w", key, ErrUnsupportedType)
			}
			encoded, err := encodeNested(values[i])
			if err != nil {
				return nil, fmt.Errorf("map entry %d: %w", i, err)
			}
			entries[i] = MapEntry{Key: stringKey, Value: encoded}
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("DuckDB value %T: %w", value, ErrUnsupportedType)
	}
}

func encodeFloat(value any) (any, error) {
	switch floating := value.(type) {
	case float32:
		if math.IsNaN(float64(floating)) || math.IsInf(float64(floating), 0) || math.Signbit(float64(floating)) && floating == 0 {
			return fmt.Sprintf("0x%08x", math.Float32bits(floating)), nil
		}
		return floating, nil
	case float64:
		if math.IsNaN(floating) || math.IsInf(floating, 0) || math.Signbit(floating) && floating == 0 {
			return fmt.Sprintf("0x%016x", math.Float64bits(floating)), nil
		}
		return floating, nil
	default:
		return nil, fmt.Errorf("expected floating-point value, got %T", value)
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
