package util

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// EncodeValue renders one received OTel value as the canonical recursive wire
// representation. Each map keeps the last received value per key, then sorts
// keys for identity. Slices retain their received order and repeated elements.
func EncodeValue(v pcommon.Value) ([]byte, error) {
	value, err := encodeValuePayload(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: value.kind, Value: value.value})
}

type encodedValue struct {
	kind  string
	value any
}

func encodeValuePayload(v pcommon.Value) (encodedValue, error) {
	switch v.Type() {
	case pcommon.ValueTypeEmpty:
		return encodedValue{kind: "empty", value: nil}, nil
	case pcommon.ValueTypeStr:
		return encodedValue{kind: "string", value: v.Str()}, nil
	case pcommon.ValueTypeBool:
		return encodedValue{kind: "bool", value: v.Bool()}, nil
	case pcommon.ValueTypeInt:
		return encodedValue{kind: "int64", value: strconv.FormatInt(v.Int(), 10)}, nil
	case pcommon.ValueTypeDouble:
		n := v.Double()
		bits := math.Float64bits(n)
		if n == 0 && math.Signbit(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return encodedValue{kind: "double", value: fmt.Sprintf("0x%016x", bits)}, nil
		}
		return encodedValue{kind: "double", value: n}, nil
	case pcommon.ValueTypeBytes:
		return encodedValue{kind: "bytes", value: base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())}, nil
	case pcommon.ValueTypeSlice:
		slice := v.Slice()
		values := make([]json.RawMessage, 0, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			child, err := EncodeValue(slice.At(i))
			if err != nil {
				return encodedValue{}, err
			}
			values = append(values, child)
		}
		return encodedValue{kind: "array", value: values}, nil
	case pcommon.ValueTypeMap:
		type mapEntry struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		latest := LastValues(v.Map())
		keys := make([]string, 0, len(latest))
		for key := range latest {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]mapEntry, 0, len(keys))
		for _, key := range keys {
			encoded, err := EncodeValue(latest[key])
			if err != nil {
				return encodedValue{}, err
			}
			values = append(values, mapEntry{Key: key, Value: encoded})
		}
		return encodedValue{kind: "map", value: values}, nil
	default:
		return encodedValue{}, fmt.Errorf("unsupported OpenTelemetry value kind %d", v.Type())
	}
}

// LastValues selects within this one collection only, without changing pdata.
// Returned values refer to the input and must not outlive or mutate it.
func LastValues(attrs pcommon.Map) map[string]pcommon.Value {
	latest := make(map[string]pcommon.Value, attrs.Len())
	for key, value := range attrs.All() {
		latest[key] = value
	}
	return latest
}

// LastValue matches the collection's last-occurrence storage rule for derived
// fields without allocating a complete map. A missing value must not be read.
func LastValue(attrs pcommon.Map, key string) (pcommon.Value, bool) {
	var value pcommon.Value
	found := false
	for candidate, current := range attrs.All() {
		if candidate == key {
			value, found = current, true
		}
	}
	return value, found
}

// SpanIDUint64 converts an OTLP 8-byte span ID to its native integer value.
func SpanIDUint64(id [8]byte) uint64 {
	return binary.BigEndian.Uint64(id[:])
}

// SpanIDWire renders a native span ID in canonical OTLP wire form.
func SpanIDWire(id uint64) string {
	return fmt.Sprintf("%016x", id)
}

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
