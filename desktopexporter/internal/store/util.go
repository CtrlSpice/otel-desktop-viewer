package store

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// buildPlaceholders returns a comma-separated list of ? placeholders for SQL IN clauses.
func buildPlaceholders(count int) string {
	if count == 0 {
		return ""
	}
	marks := make([]string, count)
	for i := range count {
		marks[i] = "?"
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
