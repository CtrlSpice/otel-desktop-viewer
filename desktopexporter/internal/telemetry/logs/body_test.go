package logs

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
)

func TestBodyValue(t *testing.T) {
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
			body := Body{Data: tt.input}
			result, err := body.Value()
			assert.NoError(t, err)

			union, ok := result.(duckdb.Union)
			assert.True(t, ok, "Value() should return duckdb.Union")
			assert.Equal(t, tt.expected.Tag, union.Tag, "tag mismatch")
			assert.Equal(t, tt.expected.Value, union.Value, "value mismatch")
		})
	}
}

func TestBodyScan(t *testing.T) {
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
			var body Body
			err := body.Scan(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, body.Data)
		})
	}
}

func TestBodyScanNil(t *testing.T) {
	var body Body
	err := body.Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, body.Data)
}

func TestBodyScanInvalidType(t *testing.T) {
	var body Body
	err := body.Scan("not a duckdb.Union")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan from string")
}

func TestBodyMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string value",
			input:    "hello world",
			expected: `"hello world"`,
		},
		{
			name:     "integer value",
			input:    int64(42),
			expected: `42`,
		},
		{
			name:     "float value",
			input:    float64(3.14),
			expected: `3.14`,
		},
		{
			name:     "boolean value",
			input:    true,
			expected: `true`,
		},
		{
			name:     "map value",
			input:    map[string]any{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "array value",
			input:    []string{"one", "two"},
			expected: `["one","two"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := Body{Data: tt.input}
			result, err := body.MarshalJSON()
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestBodyUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{
			name:     "string value",
			input:    `"hello world"`,
			expected: "hello world",
		},
		{
			name:     "integer value",
			input:    `42`,
			expected: float64(42), // JSON numbers are always float64
		},
		{
			name:     "float value",
			input:    `3.14`,
			expected: float64(3.14),
		},
		{
			name:     "boolean value",
			input:    `true`,
			expected: true,
		},
		{
			name:     "map value",
			input:    `{"key":"value"}`,
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "array value",
			input:    `["one","two"]`,
			expected: []any{"one", "two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body Body
			err := body.UnmarshalJSON([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, body.Data)
		})
	}
}

// mustMarshal is a helper function that marshals a value to JSON and panics if there's an error
func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %v: %v", v, err))
	}
	return string(b)
}
