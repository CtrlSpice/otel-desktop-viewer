package util

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// CamelToSnake converts camelCase or PascalCase to snake_case (e.g. traceID -> trace_id).
func CamelToSnake(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			prevLowerOrDigit := i > 0 && (s[i-1] >= 'a' && s[i-1] <= 'z' || s[i-1] >= '0' && s[i-1] <= '9')
			if prevLowerOrDigit {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidateColumnName checks that a snake_case column name is in the allowlist.
// Returns an error if not, preventing SQL injection through column name interpolation.
func ValidateColumnName(column string, allowed map[string]struct{}) error {
	if _, ok := allowed[column]; !ok {
		return fmt.Errorf("unknown column %q", column)
	}
	return nil
}

// BuildPlaceholders returns a comma-separated list of ? placeholders for SQL IN clauses.
func BuildPlaceholders(count int) string {
	return buildPlaceholders(count, "?")
}

// BuildUUIDPlaceholders is BuildPlaceholders for comparisons against a uuid
// column, where the values arrive as strings.
//
// Measured, not assumed: an untyped parameter already casts correctly here,
// because it is a direct parameter rather than a value read out of a CTE, and
// DuckDB resolves the direction the way we want in that position. So this
// changes no behaviour today. It is written out because the direction *is*
// position-dependent -- the same comparison sourced from a CTE column casts the
// other way and silently matches nothing -- and a delete is the worst place for
// a future refactor to discover that.
//
// A plain cast rather than try_cast, deliberately differing from how
// SearchSpans types its trace id. A read can sensibly answer "not found" for a
// malformed id; a delete that quietly removes nothing and reports success is
// the failure mode this codebase keeps getting bitten by. Garbage in should
// raise, and both the untyped and cast forms do:
//
//	                 valid id   wire form   malformed
//	untyped ?        deletes    deletes     error
//	?::uuid          deletes    deletes     error
//	try_cast(...)    deletes    deletes     0 rows, "success"
func BuildUUIDPlaceholders(count int) string {
	return buildPlaceholders(count, "?::uuid")
}

func buildPlaceholders(count int, mark string) string {
	if count == 0 {
		return ""
	}
	marks := make([]string, count)
	for i := range count {
		marks[i] = mark
	}
	return strings.Join(marks, ",")
}

// ValueToStringAndType serializes a pcommon.Value to a string and returns a type tag.
// Used for both the attributes table (Key/Value/Type) and the logs table (Body/BodyType).
func ValueToStringAndType(v pcommon.Value) (valueStr string, typeStr string) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str(), "string"
	case pcommon.ValueTypeInt:
		return strconv.FormatInt(v.Int(), 10), "int64"
	case pcommon.ValueTypeDouble:
		return strconv.FormatFloat(v.Double(), 'f', -1, 64), "float64"
	case pcommon.ValueTypeBool:
		return strconv.FormatBool(v.Bool()), "bool"
	case pcommon.ValueTypeBytes:
		bytes := v.Bytes()
		return hex.EncodeToString(bytes.AsRaw()), "string"
	case pcommon.ValueTypeSlice:
		return valueSliceToStringAndType(v)
	default:
		return fmt.Sprintf("%v", v.AsRaw()), "string"
	}
}

// valueSliceToStringAndType serializes a pcommon.Value slice to JSON array string and type.
func valueSliceToStringAndType(v pcommon.Value) (valueStr string, typeStr string) {
	slice := v.Slice()
	if slice.Len() == 0 {
		return "[]", "string[]"
	}

	firstItem := slice.At(0)
	switch firstItem.Type() {
	case pcommon.ValueTypeStr:
		typeStr = "string[]"
	case pcommon.ValueTypeInt:
		typeStr = "int64[]"
	case pcommon.ValueTypeDouble:
		typeStr = "float64[]"
	case pcommon.ValueTypeBool:
		typeStr = "boolean[]"
	default:
		typeStr = "string[]"
	}

	var parts []string
	for i := 0; i < slice.Len(); i++ {
		item := slice.At(i)
		switch item.Type() {
		case pcommon.ValueTypeStr:
			parts = append(parts, `"`+strings.ReplaceAll(item.Str(), `"`, `\"`)+`"`)
		case pcommon.ValueTypeInt:
			parts = append(parts, strconv.FormatInt(item.Int(), 10))
		case pcommon.ValueTypeDouble:
			parts = append(parts, strconv.FormatFloat(item.Double(), 'f', -1, 64))
		case pcommon.ValueTypeBool:
			parts = append(parts, strconv.FormatBool(item.Bool()))
		default:
			parts = append(parts, fmt.Sprintf("%v", item.AsRaw()))
		}
	}
	return "[" + strings.Join(parts, ",") + "]", typeStr
}
