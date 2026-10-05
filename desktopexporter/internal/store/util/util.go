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
// representation. Map entries are sorted for identity because received map
// order is not semantic; slices retain their received order.
func EncodeValue(v pcommon.Value) ([]byte, error) {
	value, err := encodeValuePayload(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(EncodedValue{Kind: value.kind, Value: value.value})
}

// EncodedValue is the canonical recursive wire representation shared by stored
// OTel values and query result projections.
type EncodedValue struct {
	Kind  string `json:"kind"`
	Value any    `json:"value"`
}

// CanonicalEncodedValue returns data unchanged only when it already conforms to
// the received OTel value model. Arbitrary JSON remains a distinct value.
func CanonicalEncodedValue(data []byte) (json.RawMessage, bool) {
	raw := json.RawMessage(data)
	if !json.Valid(raw) || !validEncodedValue(raw) {
		return nil, false
	}
	return raw, true
}

func validEncodedValue(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || len(object) != 2 {
		return false
	}
	var kind string
	if err := json.Unmarshal(object["kind"], &kind); err != nil {
		return false
	}
	value, ok := object["value"]
	if !ok {
		return false
	}
	switch kind {
	case "empty":
		return strings.TrimSpace(string(value)) == "null"
	case "string":
		var text string
		return json.Unmarshal(value, &text) == nil
	case "bool":
		var boolean bool
		return json.Unmarshal(value, &boolean) == nil
	case "int64":
		var integer string
		if json.Unmarshal(value, &integer) != nil {
			return false
		}
		parsed, err := strconv.ParseInt(integer, 10, 64)
		return err == nil && strconv.FormatInt(parsed, 10) == integer
	case "double":
		return validEncodedDouble(value)
	case "bytes":
		var encoded string
		if json.Unmarshal(value, &encoded) != nil {
			return false
		}
		_, err := base64.StdEncoding.DecodeString(encoded)
		return err == nil
	case "array":
		var values []json.RawMessage
		if json.Unmarshal(value, &values) != nil {
			return false
		}
		for _, child := range values {
			if !validEncodedValue(child) {
				return false
			}
		}
		return true
	case "map":
		var entries []map[string]json.RawMessage
		if json.Unmarshal(value, &entries) != nil {
			return false
		}
		for _, entry := range entries {
			if len(entry) != 2 || !validEncodedValue(entry["value"]) {
				return false
			}
			var key string
			if json.Unmarshal(entry["key"], &key) != nil {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validEncodedDouble(raw json.RawMessage) bool {
	var bits string
	if json.Unmarshal(raw, &bits) == nil {
		if len(bits) != 18 || !strings.HasPrefix(bits, "0x") {
			return false
		}
		_, err := strconv.ParseUint(bits[2:], 16, 64)
		return err == nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var number json.Number
	if decoder.Decode(&number) != nil {
		return false
	}
	_, err := strconv.ParseFloat(number.String(), 64)
	return err == nil
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
		values := make([]mapEntry, 0, v.Map().Len())
		var encodeErr error
		v.Map().Range(func(key string, child pcommon.Value) bool {
			encoded, err := EncodeValue(child)
			if err != nil {
				encodeErr = err
				return false
			}
			values = append(values, mapEntry{Key: key, Value: encoded})
			return true
		})
		if encodeErr != nil {
			return encodedValue{}, encodeErr
		}
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].Key != values[j].Key {
				return values[i].Key < values[j].Key
			}
			return string(values[i].Value) < string(values[j].Value)
		})
		return encodedValue{kind: "map", value: values}, nil
	default:
		return encodedValue{}, fmt.Errorf("unsupported OpenTelemetry value kind %d", v.Type())
	}
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
