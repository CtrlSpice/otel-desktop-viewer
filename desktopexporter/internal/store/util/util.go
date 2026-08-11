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

// UUIDList renders a set of uuid strings as one bound-list predicate body.
//
// The SQL it goes into is static: `where id in (` + UUIDList() + `)`, with the
// whole set travelling as a single argument. That removes the class of bug the
// previous form invited, where the number of `?::uuid` placeholders built into
// the text and the number of arguments appended beside it could drift apart --
// a mismatch SQL cannot catch, because both counts are correct on their own.
//
// The list binds as varchar[] and is cast inside the query rather than bound as
// uuid[]. That is not a stylistic choice: binding the driver's duckdb.UUID type
// as a *parameter* silently returns nothing. Measured -- a bound
// 11111111-1111-1111-1111-111111111111 reads back as 91111111-..., matching no
// row and raising no error, for both a scalar parameter and a list. It is safe
// through the appender, which is the only place this codebase uses the type,
// and every query path already binds uuids as strings.
//
// The likely cause, offered as a lead rather than a fact: DuckDB documents
// UUIDs as "represented internally as HUGEINT values", which is signed, so
// something has to flip the top bit to keep unsigned UUID ordering intact --
// and the corruption observed is exactly bit 127. That flip is not documented
// and is not visible from SQL (UUIDs sort 0, 1, 9, f as you would expect, and
// the UUID -> HUGEINT cast is not implemented), so treat the mechanism as
// unconfirmed. The behaviour is not: do not bind duckdb.UUID as a parameter.
//
// Watch this. The double cast is a workaround for a driver limitation, not a
// property of the schema, so it should not outlive the limitation.
// TestDriverStillCannotBindUUIDLists pins both halves and fails the moment
// either lifts -- at which point this collapses to `select unnest(?::uuid[])`
// and the comment above can go with it.
//
// Measured at 204,891 spans, deletes rolled back between rounds: within noise
// of the placeholder form at every size (0.3ms vs 0.5ms at one id, 22.4ms vs
// 21.8ms at a thousand), so the safety costs nothing worth counting.
func UUIDList() string {
	return "select unnest(?::varchar[])::uuid"
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

// ToStringList converts the []any of ids the JSON-RPC layer hands us into the
// []string the driver binds as varchar[].
//
// The ids arrive as any because they come from decoded JSON; the handler has
// already validated each one parses as a uuid, so anything non-string here
// would be a programming error rather than bad input, and is dropped rather
// than silently stringified into something that cannot match.
func ToStringList(ids []any) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if s, ok := id.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
