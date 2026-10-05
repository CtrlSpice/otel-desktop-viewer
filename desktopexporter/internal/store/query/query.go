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

	storeutil "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

const DefaultLimit uint64 = 25

var (
	ErrReadOnly        = errors.New("query must be one read-only SELECT or supported introspection statement")
	ErrUnsupportedType = errors.New("query result type has no approved lossless wire representation")
)

type Column struct {
	Name       string `json:"name"`
	DuckDBType string `json:"duckdbType"`
}

type MapEntry struct {
	Key   any `json:"key"`
	Value any `json:"value"`
}

type Value = storeutil.EncodedValue

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
		result.Columns[i] = Column{Name: originalNames[i], DuckDBType: databaseType}
	}

	withLookahead := limit != ^uint64(0)
	executionSQL, err := executionQuery(sourceNames, databaseTypes, withLookahead)
	if err != nil {
		return result, fmt.Errorf("prepare lossless query projection: %w", err)
	}
	var rows *sql.Rows
	if withLookahead {
		rows, err = conn.QueryContext(ctx, executionSQL, statement, limit+1)
	} else {
		rows, err = conn.QueryContext(ctx, executionSQL, statement)
	}
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

func executionQuery(names, types []string, withLimit bool) (string, error) {
	projections := make([]string, len(names))
	lambdaID := 0
	for i, name := range names {
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		logicalType, err := parseLogicalType(types[i])
		if err != nil {
			return "", fmt.Errorf("column %q type %q: %w", name, types[i], err)
		}
		projection := quoted
		if needsLosslessProjection(logicalType) {
			projection = losslessProjection(quoted, logicalType, &lambdaID)
		}
		projections[i] = projection + " AS " + quoted
	}
	query := "SELECT " + strings.Join(projections, ", ") + " FROM query(?)"
	if withLimit {
		query += " LIMIT ?"
	}
	return query, nil
}

type logicalTypeKind uint8

const (
	logicalScalar logicalTypeKind = iota
	logicalJSON
	logicalList
	logicalMap
	logicalStruct
)

type logicalType struct {
	kind   logicalTypeKind
	name   string
	child  *logicalType
	key    *logicalType
	value  *logicalType
	fields []logicalField
}

type logicalField struct {
	name  string
	type_ logicalType
}

func parseLogicalType(typeName string) (logicalType, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return logicalType{}, errors.New("empty logical type")
	}
	if base, ok := stripListOrArraySuffix(typeName); ok {
		child, err := parseLogicalType(base)
		return logicalType{kind: logicalList, child: &child}, err
	}
	if strings.EqualFold(typeName, "JSON") {
		return logicalType{kind: logicalJSON, name: "JSON"}, nil
	}
	if inner, ok := typeArguments(typeName, "MAP"); ok {
		parts, err := splitTypeList(inner)
		if err != nil || len(parts) != 2 {
			return logicalType{}, fmt.Errorf("invalid MAP type")
		}
		key, err := parseLogicalType(parts[0])
		if err != nil {
			return logicalType{}, err
		}
		value, err := parseLogicalType(parts[1])
		return logicalType{kind: logicalMap, key: &key, value: &value}, err
	}
	if inner, ok := typeArguments(typeName, "STRUCT"); ok {
		parts, err := splitTypeList(inner)
		if err != nil {
			return logicalType{}, err
		}
		fields := make([]logicalField, len(parts))
		for i, part := range parts {
			name, childType, err := splitStructField(part)
			if err != nil {
				return logicalType{}, err
			}
			child, err := parseLogicalType(childType)
			if err != nil {
				return logicalType{}, err
			}
			fields[i] = logicalField{name: name, type_: child}
		}
		return logicalType{kind: logicalStruct, fields: fields}, nil
	}
	return logicalType{kind: logicalScalar, name: strings.ToUpper(typeName)}, nil
}

func stripListOrArraySuffix(typeName string) (string, bool) {
	if !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	depth := 0
	quoted := false
	lastOpen := -1
	for i := 0; i < len(typeName); i++ {
		switch typeName[i] {
		case '"':
			if quoted && i+1 < len(typeName) && typeName[i+1] == '"' {
				i++
				continue
			}
			quoted = !quoted
		case '(':
			if !quoted {
				depth++
			}
		case ')':
			if !quoted {
				depth--
			}
		case '[':
			if !quoted && depth == 0 {
				lastOpen = i
			}
		}
	}
	if quoted || depth != 0 || lastOpen <= 0 {
		return "", false
	}
	return strings.TrimSpace(typeName[:lastOpen]), true
}

func typeArguments(typeName, constructor string) (string, bool) {
	prefix := constructor + "("
	if len(typeName) <= len(prefix) || !strings.EqualFold(typeName[:len(prefix)], prefix) || typeName[len(typeName)-1] != ')' {
		return "", false
	}
	return typeName[len(prefix) : len(typeName)-1], true
}

func splitTypeList(input string) ([]string, error) {
	var parts []string
	start, parenDepth, bracketDepth := 0, 0, 0
	quoted := false
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '"':
			if quoted && i+1 < len(input) && input[i+1] == '"' {
				i++
				continue
			}
			quoted = !quoted
		case '(':
			if !quoted {
				parenDepth++
			}
		case ')':
			if !quoted {
				parenDepth--
			}
		case '[':
			if !quoted {
				bracketDepth++
			}
		case ']':
			if !quoted {
				bracketDepth--
			}
		case ',':
			if !quoted && parenDepth == 0 && bracketDepth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				start = i + 1
			}
		}
		if parenDepth < 0 || bracketDepth < 0 {
			return nil, errors.New("unbalanced logical type")
		}
	}
	if quoted || parenDepth != 0 || bracketDepth != 0 {
		return nil, errors.New("unbalanced logical type")
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts, nil
}

func splitStructField(field string) (string, string, error) {
	field = strings.TrimSpace(field)
	if field == "" {
		return "", "", errors.New("empty STRUCT field")
	}
	if field[0] != '"' {
		space := strings.IndexAny(field, " \t")
		if space <= 0 {
			return "", "", fmt.Errorf("invalid STRUCT field %q", field)
		}
		return field[:space], strings.TrimSpace(field[space:]), nil
	}
	var name strings.Builder
	for i := 1; i < len(field); i++ {
		if field[i] != '"' {
			name.WriteByte(field[i])
			continue
		}
		if i+1 < len(field) && field[i+1] == '"' {
			name.WriteByte('"')
			i++
			continue
		}
		childType := strings.TrimSpace(field[i+1:])
		if childType == "" {
			return "", "", fmt.Errorf("STRUCT field %q has no type", name.String())
		}
		return name.String(), childType, nil
	}
	return "", "", fmt.Errorf("unterminated STRUCT field %q", field)
}

func needsLosslessProjection(type_ logicalType) bool {
	switch type_.kind {
	case logicalJSON, logicalMap:
		return true
	case logicalScalar:
		return isTemporalType(type_.name) || type_.name == "INTERVAL"
	case logicalList:
		return needsLosslessProjection(*type_.child)
	case logicalStruct:
		for _, field := range type_.fields {
			if needsLosslessProjection(field.type_) {
				return true
			}
		}
	}
	return false
}

func losslessProjection(expression string, type_ logicalType, lambdaID *int) string {
	switch type_.kind {
	case logicalJSON:
		return "CAST(" + expression + " AS VARCHAR)"
	case logicalList:
		name := fmt.Sprintf("item_%d", *lambdaID)
		*lambdaID = *lambdaID + 1
		return "list_transform(" + expression + ", " + name + " -> " + losslessProjection(name, *type_.child, lambdaID) + ")"
	case logicalMap:
		name := fmt.Sprintf("entry_%d", *lambdaID)
		*lambdaID = *lambdaID + 1
		key := losslessProjection(name+`."key"`, *type_.key, lambdaID)
		value := losslessProjection(name+`."value"`, *type_.value, lambdaID)
		return "list_transform(map_entries(" + expression + "), " + name + " -> struct_pack(key := " + key + ", value := " + value + "))"
	case logicalStruct:
		fields := make([]string, len(type_.fields))
		for i, field := range type_.fields {
			quoted := `"` + strings.ReplaceAll(field.name, `"`, `""`) + `"`
			fields[i] = quoted + " := " + losslessProjection(expression+"."+quoted, field.type_, lambdaID)
		}
		return "CASE WHEN " + expression + " IS NULL THEN NULL ELSE struct_pack(" + strings.Join(fields, ", ") + ") END"
	case logicalScalar:
		if isTemporalType(type_.name) || type_.name == "INTERVAL" {
			return "CAST(" + expression + " AS VARCHAR)"
		}
		return expression
	default:
		return expression
	}
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

func isIntegerType(typeName string) bool {
	switch typeName {
	case "TINYINT", "SMALLINT", "INTEGER", "BIGINT", "HUGEINT", "BIGNUM",
		"UTINYINT", "USMALLINT", "UINTEGER", "UBIGINT", "UHUGEINT":
		return true
	default:
		return false
	}
}

func isSignedInt64Type(typeName string) bool {
	switch typeName {
	case "TINYINT", "SMALLINT", "INTEGER", "BIGINT":
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
	type_, err := parseLogicalType(column.DuckDBType)
	if err != nil {
		return nil, err
	}
	return encodeTypedValue(type_, value)
}

func encodeTypedValue(type_ logicalType, value any) (any, error) {
	if value == nil {
		return Value{Kind: "empty", Value: nil}, nil
	}
	switch type_.kind {
	case logicalJSON:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("expected JSON text, got %T", value)
		}
		if raw, ok := storeutil.CanonicalEncodedValue([]byte(text)); ok {
			return raw, nil
		}
		return Value{Kind: "string", Value: text}, nil
	case logicalList:
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("expected LIST or ARRAY, got %T", value)
		}
		encoded := make([]any, len(items))
		for i, item := range items {
			var err error
			encoded[i], err = encodeTypedValue(*type_.child, item)
			if err != nil {
				return nil, fmt.Errorf("list item %d: %w", i, err)
			}
		}
		return Value{Kind: "array", Value: encoded}, nil
	case logicalMap:
		return encodeMapValue(type_, value)
	case logicalStruct:
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected STRUCT, got %T", value)
		}
		entries := make([]MapEntry, len(type_.fields))
		for i, field := range type_.fields {
			fieldValue, present := fields[field.name]
			if !present {
				return nil, fmt.Errorf("STRUCT field %q is missing", field.name)
			}
			encoded, err := encodeTypedValue(field.type_, fieldValue)
			if err != nil {
				return nil, fmt.Errorf("STRUCT field %q: %w", field.name, err)
			}
			entries[i] = MapEntry{Key: field.name, Value: encoded}
		}
		return Value{Kind: "map", Value: entries}, nil
	default:
		return encodeScalarValue(type_.name, value)
	}
}

func encodeMapValue(type_ logicalType, value any) (Value, error) {
	items, ok := value.([]any)
	if !ok {
		return Value{}, fmt.Errorf("expected MAP entries, got %T", value)
	}
	entries := make([]MapEntry, len(items))
	for i, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return Value{}, fmt.Errorf("map entry %d: expected STRUCT, got %T", i, item)
		}
		key, keyPresent := fields["key"]
		entryValue, valuePresent := fields["value"]
		if !keyPresent || !valuePresent {
			return Value{}, fmt.Errorf("map entry %d: missing key or value", i)
		}
		encodedKey, err := encodeTypedValue(*type_.key, key)
		if err != nil {
			return Value{}, fmt.Errorf("map entry %d key: %w", i, err)
		}
		encodedValue, err := encodeTypedValue(*type_.value, entryValue)
		if err != nil {
			return Value{}, fmt.Errorf("map entry %d value: %w", i, err)
		}
		entries[i] = MapEntry{Key: encodedKey, Value: encodedValue}
	}
	return Value{Kind: "map", Value: entries}, nil
}

func encodeScalarValue(typeName string, value any) (Value, error) {
	switch {
	case typeName == "BOOLEAN":
		boolean, ok := value.(bool)
		if !ok {
			return Value{}, fmt.Errorf("expected BOOLEAN, got %T", value)
		}
		return Value{Kind: "bool", Value: boolean}, nil
	case isSignedInt64Type(typeName):
		integer, err := integerString(value)
		return Value{Kind: "int64", Value: integer}, err
	case isIntegerType(typeName):
		integer, err := integerString(value)
		return Value{Kind: "string", Value: integer}, err
	case strings.HasPrefix(typeName, "DECIMAL("):
		decimal, ok := value.(duckdb.Decimal)
		if !ok {
			return Value{}, fmt.Errorf("expected DECIMAL, got %T", value)
		}
		return Value{Kind: "string", Value: fixedDecimal(decimal)}, nil
	case typeName == "FLOAT" || typeName == "DOUBLE":
		floating, err := encodeFloat(value)
		return Value{Kind: "double", Value: floating}, err
	case typeName == "UUID":
		bytes, ok := value.([]byte)
		if !ok {
			return Value{}, fmt.Errorf("expected UUID bytes, got %T", value)
		}
		id, err := uuid.FromBytes(bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: "string", Value: id.String()}, nil
	case typeName == "BLOB" || typeName == "GEOMETRY":
		bytes, ok := value.([]byte)
		if !ok {
			return Value{}, fmt.Errorf("expected bytes, got %T", value)
		}
		return Value{Kind: "bytes", Value: base64.StdEncoding.EncodeToString(bytes)}, nil
	case typeName == "VARCHAR" || typeName == "ENUM" || typeName == "BIT" ||
		isTemporalType(typeName) || typeName == "INTERVAL":
		text, ok := value.(string)
		if !ok {
			return Value{}, fmt.Errorf("expected %s text, got %T", typeName, value)
		}
		return Value{Kind: "string", Value: text}, nil
	default:
		return Value{}, fmt.Errorf("DuckDB type %s: %w", typeName, ErrUnsupportedType)
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
