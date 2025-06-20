package store

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
	"github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
)

// mustMarshal is a helper function that marshals a value to JSON and panics if there's an error
func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %v: %v", v, err))
	}
	return string(b)
}

func TestStringifyOnOverflow(t *testing.T) {
	tests := []struct {
		name             string
		attributeName    string
		values           []uint64
		expectedValue    any
		expectedOverflow bool
	}{
		{
			name:             "single value no overflow",
			attributeName:    "test_attr",
			values:           []uint64{100},
			expectedValue:    int64(100),
			expectedOverflow: false,
		},
		{
			name:             "single value with overflow",
			attributeName:    "test_attr",
			values:           []uint64{math.MaxUint64},
			expectedValue:    "18446744073709551615",
			expectedOverflow: true,
		},
		{
			name:             "single value at boundary (no overflow)",
			attributeName:    "test_attr",
			values:           []uint64{math.MaxInt64},
			expectedValue:    int64(math.MaxInt64),
			expectedOverflow: false,
		},
		{
			name:             "single value just over boundary",
			attributeName:    "test_attr",
			values:           []uint64{math.MaxInt64 + 1},
			expectedValue:    "9223372036854775808",
			expectedOverflow: true,
		},
		{
			name:             "slice no overflow",
			attributeName:    "test_attr",
			values:           []uint64{100, 200, 300},
			expectedValue:    []int64{100, 200, 300},
			expectedOverflow: false,
		},
		{
			name:             "slice with overflow",
			attributeName:    "test_attr",
			values:           []uint64{100, math.MaxUint64, 300},
			expectedValue:    []string{"100", "18446744073709551615", "300"},
			expectedOverflow: true,
		},
		{
			name:             "slice all overflow values",
			attributeName:    "test_attr",
			values:           []uint64{math.MaxUint64, math.MaxInt64 + 1},
			expectedValue:    []string{"18446744073709551615", "9223372036854775808"},
			expectedOverflow: true,
		},
		{
			name:             "empty slice",
			attributeName:    "test_attr",
			values:           []uint64{},
			expectedValue:    []int64{},
			expectedOverflow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, hasOverflow := stringifyOnOverflow(tt.attributeName, tt.values...)
			assert.Equal(t, tt.expectedOverflow, hasOverflow, "overflow detection mismatch")
			assert.Equal(t, tt.expectedValue, result, "returned value mismatch")
		})
	}
}

func TestFromDbMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]duckdb.Union
		expected map[string]any
	}{
		{
			name:     "empty map",
			input:    map[string]duckdb.Union{},
			expected: map[string]any{},
		},
		{
			name: "simple attributes",
			input: map[string]duckdb.Union{
				"string":  {Value: "value"},
				"int":     {Value: int64(42)},
				"float":   {Value: float64(3.14)},
				"bool":    {Value: true},
				"strings": {Value: []string{"a", "b", "c"}},
			},
			expected: map[string]any{
				"string":  "value",
				"int":     int64(42),
				"float":   float64(3.14),
				"bool":    true,
				"strings": []string{"a", "b", "c"},
			},
		},
		{
			name: "various attribute types",
			input: map[string]duckdb.Union{
				"string":  {Tag: "str", Value: "hello"},
				"int":     {Tag: "bigint", Value: int64(42)},
				"float":   {Tag: "double", Value: float64(3.14)},
				"bool":    {Tag: "boolean", Value: true},
				"strings": {Tag: "str_list", Value: []string{"a", "b", "c"}},
				"ints":    {Tag: "bigint_list", Value: []int64{1, 2, 3}},
				"floats":  {Tag: "double_list", Value: []float64{1.1, 2.2, 3.3}},
				"bools":   {Tag: "boolean_list", Value: []bool{true, false, true}},
			},
			expected: map[string]any{
				"string":  "hello",
				"int":     int64(42),
				"float":   float64(3.14),
				"bool":    true,
				"strings": []string{"a", "b", "c"},
				"ints":    []int64{1, 2, 3},
				"floats":  []float64{1.1, 2.2, 3.3},
				"bools":   []bool{true, false, true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromDbMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToDbMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]duckdb.Union
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]duckdb.Union{},
		},
		{
			name: "simple attributes",
			input: map[string]any{
				"string":  "value",
				"int":     int64(42),
				"float":   float64(3.14),
				"bool":    true,
				"strings": []string{"a", "b", "c"},
			},
			expected: map[string]duckdb.Union{
				"string":  {Tag: "str", Value: "value"},
				"int":     {Tag: "bigint", Value: int64(42)},
				"float":   {Tag: "double", Value: float64(3.14)},
				"bool":    {Tag: "boolean", Value: true},
				"strings": {Tag: "str_list", Value: []string{"a", "b", "c"}},
			},
		},
		{
			name: "uint64 overflow handling",
			input: map[string]any{
				"safe_uint64":     uint64(100),
				"overflow_uint64": uint64(math.MaxUint64),
				"safe_slice":      []uint64{100, 200, 300},
				"overflow_slice":  []uint64{100, math.MaxUint64, 300},
			},
			expected: map[string]duckdb.Union{
				"safe_uint64":     {Tag: "bigint", Value: int64(100)},
				"overflow_uint64": {Tag: "str", Value: "18446744073709551615"},
				"safe_slice":      {Tag: "bigint_list", Value: []int64{100, 200, 300}},
				"overflow_slice":  {Tag: "str_list", Value: []string{"100", "18446744073709551615", "300"}},
			},
		},
		{
			name: "mixed type arrays",
			input: map[string]any{
				"mixed_ints":   []any{int(1), int32(2), int64(3)},
				"mixed_floats": []any{float32(1.1), float64(2.2)},
				"mixed_types":  []any{"string", int64(42), float64(3.14)},
			},
			expected: map[string]duckdb.Union{
				"mixed_ints":   {Tag: "bigint_list", Value: []any{int(1), int32(2), int64(3)}},
				"mixed_floats": {Tag: "double_list", Value: []any{float32(1.1), float64(2.2)}},
				"mixed_types":  {Tag: "str_list", Value: []string{"string", "42", "3.14"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toDbMap(tt.input)

			// Compare each union separately for better error messages
			assert.Equal(t, len(tt.expected), len(result), "map length mismatch")

			for key, expectedUnion := range tt.expected {
				actualValue, exists := result[key]
				assert.True(t, exists, "key %s not found in result", key)

				// Cast the actual value to duckdb.Union for comparison
				actualUnion, ok := actualValue.(duckdb.Union)
				assert.True(t, ok, "value for key %s is not a duckdb.Union", key)

				assert.Equal(t, expectedUnion.Tag, actualUnion.Tag, "tag mismatch for key %s", key)
				assert.Equal(t, expectedUnion.Value, actualUnion.Value, "value mismatch for key %s", key)
			}
		})
	}
}

func TestToDbEvents(t *testing.T) {
	now := time.Now().UnixNano()
	tests := []struct {
		name     string
		input    []traces.EventData
		expected []dbEvent
	}{
		{
			name:     "empty events",
			input:    []traces.EventData{},
			expected: []dbEvent{},
		},
		{
			name: "single event",
			input: []traces.EventData{
				{
					Name:                   "test event",
					Timestamp:              now,
					Attributes:             map[string]any{"key": "value"},
					DroppedAttributesCount: 0,
				},
			},
			expected: []dbEvent{
				{
					Name:                   "test event",
					Timestamp:              now,
					Attributes:             duckdb.Map{"key": duckdb.Union{Tag: "str", Value: "value"}},
					DroppedAttributesCount: 0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toDbEvents(tt.input)
			assert.Equal(t, len(tt.expected), len(result))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name)
				assert.Equal(t, expected.Timestamp, result[i].Timestamp)
				assert.Equal(t, expected.DroppedAttributesCount, result[i].DroppedAttributesCount)
				// Compare attributes separately for better error messages
				assert.Equal(t, len(expected.Attributes), len(result[i].Attributes))
			}
		})
	}
}

func TestToDbLinks(t *testing.T) {
	tests := []struct {
		name     string
		input    []traces.LinkData
		expected []dbLink
	}{
		{
			name:     "empty links",
			input:    []traces.LinkData{},
			expected: []dbLink{},
		},
		{
			name: "single link",
			input: []traces.LinkData{
				{
					TraceID:                "trace1",
					SpanID:                 "span1",
					TraceState:             "state1",
					Attributes:             map[string]any{"key": "value"},
					DroppedAttributesCount: 0,
				},
			},
			expected: []dbLink{
				{
					TraceID:                "trace1",
					SpanID:                 "span1",
					TraceState:             "state1",
					Attributes:             duckdb.Map{"key": duckdb.Union{Tag: "str", Value: "value"}},
					DroppedAttributesCount: 0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toDbLinks(tt.input)
			assert.Equal(t, len(tt.expected), len(result))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.TraceID, result[i].TraceID)
				assert.Equal(t, expected.SpanID, result[i].SpanID)
				assert.Equal(t, expected.TraceState, result[i].TraceState)
				assert.Equal(t, expected.DroppedAttributesCount, result[i].DroppedAttributesCount)
				// Compare attributes separately for better error messages
				assert.Equal(t, len(expected.Attributes), len(result[i].Attributes))
			}
		})
	}
}

func TestFromDbEvents(t *testing.T) {
	now := time.Now().UnixNano()
	tests := []struct {
		name     string
		input    []dbEvent
		expected []traces.EventData
	}{
		{
			name:     "empty events",
			input:    []dbEvent{},
			expected: []traces.EventData{},
		},
		{
			name: "single event with attributes",
			input: []dbEvent{
				{
					Name:      "test event",
					Timestamp: now,
					Attributes: duckdb.Map{
						"string_attr": duckdb.Union{Tag: "str", Value: "hello"},
						"int_attr":    duckdb.Union{Tag: "bigint", Value: int64(42)},
						"bool_attr":   duckdb.Union{Tag: "boolean", Value: true},
					},
					DroppedAttributesCount: 1,
				},
			},
			expected: []traces.EventData{
				{
					Name:      "test event",
					Timestamp: now,
					Attributes: map[string]any{
						"string_attr": "hello",
						"int_attr":    int64(42),
						"bool_attr":   true,
					},
					DroppedAttributesCount: 1,
				},
			},
		},
		{
			name: "multiple events",
			input: []dbEvent{
				{
					Name:                   "event1",
					Timestamp:              now,
					Attributes:             duckdb.Map{"key1": duckdb.Union{Tag: "str", Value: "value1"}},
					DroppedAttributesCount: 0,
				},
				{
					Name:                   "event2",
					Timestamp:              now + time.Second.Nanoseconds(),
					Attributes:             duckdb.Map{"key2": duckdb.Union{Tag: "bigint", Value: int64(100)}},
					DroppedAttributesCount: 2,
				},
			},
			expected: []traces.EventData{
				{
					Name:                   "event1",
					Timestamp:              now,
					Attributes:             map[string]any{"key1": "value1"},
					DroppedAttributesCount: 0,
				},
				{
					Name:                   "event2",
					Timestamp:              now + time.Second.Nanoseconds(),
					Attributes:             map[string]any{"key2": int64(100)},
					DroppedAttributesCount: 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromDbEvents(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFromDbLinks(t *testing.T) {
	tests := []struct {
		name     string
		input    []dbLink
		expected []traces.LinkData
	}{
		{
			name:     "empty links",
			input:    []dbLink{},
			expected: []traces.LinkData{},
		},
		{
			name: "single link with attributes",
			input: []dbLink{
				{
					TraceID:    "trace123",
					SpanID:     "span456",
					TraceState: "state1",
					Attributes: duckdb.Map{
						"link_attr": duckdb.Union{Tag: "str", Value: "link_value"},
						"num_attr":  duckdb.Union{Tag: "double", Value: float64(3.14)},
					},
					DroppedAttributesCount: 0,
				},
			},
			expected: []traces.LinkData{
				{
					TraceID:    "trace123",
					SpanID:     "span456",
					TraceState: "state1",
					Attributes: map[string]any{
						"link_attr": "link_value",
						"num_attr":  float64(3.14),
					},
					DroppedAttributesCount: 0,
				},
			},
		},
		{
			name: "multiple links",
			input: []dbLink{
				{
					TraceID:                "trace1",
					SpanID:                 "span1",
					TraceState:             "state1",
					Attributes:             duckdb.Map{"attr1": duckdb.Union{Tag: "boolean", Value: false}},
					DroppedAttributesCount: 1,
				},
				{
					TraceID:                "trace2",
					SpanID:                 "span2",
					TraceState:             "state2",
					Attributes:             duckdb.Map{"attr2": duckdb.Union{Tag: "str_list", Value: []string{"a", "b"}}},
					DroppedAttributesCount: 3,
				},
			},
			expected: []traces.LinkData{
				{
					TraceID:                "trace1",
					SpanID:                 "span1",
					TraceState:             "state1",
					Attributes:             map[string]any{"attr1": false},
					DroppedAttributesCount: 1,
				},
				{
					TraceID:                "trace2",
					SpanID:                 "span2",
					TraceState:             "state2",
					Attributes:             map[string]any{"attr2": []string{"a", "b"}},
					DroppedAttributesCount: 3,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromDbLinks(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToDbBody(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected duckdb.Union
	}{
		{
			name:     "string value",
			input:    "hello world",
			expected: duckdb.Union{Tag: "str", Value: "hello world"},
		},
		{
			name:     "integer value",
			input:    int64(42),
			expected: duckdb.Union{Tag: "bigint", Value: int64(42)},
		},
		{
			name:     "float value",
			input:    float64(3.14159),
			expected: duckdb.Union{Tag: "double", Value: float64(3.14159)},
		},
		{
			name:     "float32 value",
			input:    float32(3.14),
			expected: duckdb.Union{Tag: "double", Value: float32(3.14)},
		},
		{
			name:     "boolean value",
			input:    true,
			expected: duckdb.Union{Tag: "boolean", Value: true},
		},
		{
			name:     "byte array",
			input:    []byte("binary data"),
			expected: duckdb.Union{Tag: "bytes", Value: []byte("binary data")},
		},
		{
			name:     "safe uint64",
			input:    uint64(100),
			expected: duckdb.Union{Tag: "bigint", Value: int64(100)},
		},
		{
			name:     "overflow uint64",
			input:    uint64(math.MaxUint64),
			expected: duckdb.Union{Tag: "str", Value: "18446744073709551615"},
		},
		{
			name:     "complex struct",
			input:    struct{ Name string }{Name: "test"},
			expected: duckdb.Union{Tag: "json", Value: mustMarshal(struct{ Name string }{Name: "test"})},
		},
		{
			name:     "string array",
			input:    []string{"one", "two", "three"},
			expected: duckdb.Union{Tag: "json", Value: mustMarshal([]string{"one", "two", "three"})},
		},
		{
			name:     "mixed array",
			input:    []any{"string", 42, true},
			expected: duckdb.Union{Tag: "json", Value: mustMarshal([]any{"string", 42, true})},
		},
		{
			name:     "nested map",
			input:    map[string]any{"key": "value", "nested": map[string]any{"inner": 42}},
			expected: duckdb.Union{Tag: "json", Value: mustMarshal(map[string]any{"key": "value", "nested": map[string]any{"inner": 42}})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toDbBody(tt.input)
			assert.Equal(t, tt.expected.Tag, result.Tag, "tag mismatch")
			assert.Equal(t, tt.expected.Value, result.Value, "value mismatch")
		})
	}
}

func TestFromDbBody(t *testing.T) {
	tests := []struct {
		name     string
		input    duckdb.Union
		expected any
	}{
		{
			name:     "string value",
			input:    duckdb.Union{Tag: "str", Value: "hello world"},
			expected: "hello world",
		},
		{
			name:     "integer value",
			input:    duckdb.Union{Tag: "bigint", Value: int64(42)},
			expected: int64(42),
		},
		{
			name:     "float value",
			input:    duckdb.Union{Tag: "double", Value: float64(3.14159)},
			expected: float64(3.14159),
		},
		{
			name:     "boolean value",
			input:    duckdb.Union{Tag: "boolean", Value: true},
			expected: true,
		},
		{
			name:     "byte array",
			input:    duckdb.Union{Tag: "bytes", Value: []byte("binary data")},
			expected: []byte("binary data"),
		},
		{
			name:     "safe uint64 as bigint",
			input:    duckdb.Union{Tag: "bigint", Value: int64(100)},
			expected: int64(100),
		},
		{
			name:     "overflow uint64 as string",
			input:    duckdb.Union{Tag: "str", Value: "18446744073709551615"},
			expected: "18446744073709551615",
		},
		{
			name:     "json object",
			input:    duckdb.Union{Tag: "json", Value: mustMarshal(struct{ Name string }{Name: "test"})},
			expected: map[string]any{"Name": "test"},
		},
		{
			name:     "json array",
			input:    duckdb.Union{Tag: "json", Value: mustMarshal([]string{"one", "two", "three"})},
			expected: []any{"one", "two", "three"},
		},
		{
			name:     "json mixed array",
			input:    duckdb.Union{Tag: "json", Value: mustMarshal([]any{"string", 42, true})},
			expected: []any{"string", float64(42), true},
		},
		{
			name:  "json nested map",
			input: duckdb.Union{Tag: "json", Value: mustMarshal(map[string]any{"key": "value", "nested": map[string]any{"inner": 42}})},
			expected: map[string]any{
				"key": "value",
				"nested": map[string]any{
					"inner": float64(42),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromDbBody(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
