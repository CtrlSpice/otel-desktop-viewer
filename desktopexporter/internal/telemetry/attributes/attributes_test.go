package attributes

import (
	"math"
	"testing"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
)

func TestAttributesValueAndScan(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]any
		expected     map[string]any
		expectedTags map[string]string
	}{
		{
			name:         "empty map",
			input:        map[string]any{},
			expected:     map[string]any{},
			expectedTags: map[string]string{},
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
			expected: map[string]any{
				"string":  "value",
				"int":     int64(42),
				"float":   float64(3.14),
				"bool":    true,
				"strings": []string{"a", "b", "c"},
			},
			expectedTags: map[string]string{
				"string":  "str",
				"int":     "bigint",
				"float":   "double",
				"bool":    "boolean",
				"strings": "str_list",
			},
		},
		{
			name: "various attribute types",
			input: map[string]any{
				"string":  "hello",
				"int":     int64(42),
				"float":   float64(3.14),
				"bool":    true,
				"strings": []string{"a", "b", "c"},
				"ints":    []int64{1, 2, 3},
				"floats":  []float64{1.1, 2.2, 3.3},
				"bools":   []bool{true, false, true},
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
			expectedTags: map[string]string{
				"string":  "str",
				"int":     "bigint",
				"float":   "double",
				"bool":    "boolean",
				"strings": "str_list",
				"ints":    "bigint_list",
				"floats":  "double_list",
				"bools":   "boolean_list",
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
			expected: map[string]any{
				"safe_uint64":     int64(100),
				"overflow_uint64": "18446744073709551615",
				"safe_slice":      []int64{100, 200, 300},
				"overflow_slice":  []string{"100", "18446744073709551615", "300"},
			},
			expectedTags: map[string]string{
				"safe_uint64":     "bigint",
				"overflow_uint64": "str",
				"safe_slice":      "bigint_list",
				"overflow_slice":  "str_list",
			},
		},
		{
			name: "mixed type arrays",
			input: map[string]any{
				"mixed_ints":   []any{int(1), int32(2), int64(3)},
				"mixed_floats": []any{float32(1.1), float64(2.2)},
				"mixed_types":  []any{"string", int64(42), float64(3.14)},
			},
			expected: map[string]any{
				"mixed_ints":   []any{int(1), int32(2), int64(3)},
				"mixed_floats": []any{float32(1.1), float64(2.2)},
				"mixed_types":  []string{"string", "42", "3.14"},
			},
			expectedTags: map[string]string{
				"mixed_ints":   "bigint_list",
				"mixed_floats": "double_list",
				"mixed_types":  "str_list",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Value() method
			attrs := Attributes(tt.input)
			dbValue, err := attrs.Value()
			assert.NoError(t, err)

			// Verify the driver.Value is a duckdb.Map
			dbMap, ok := dbValue.(duckdb.Map)
			assert.True(t, ok, "Value() should return duckdb.Map")

			// Test Scan() method
			var scannedAttrs Attributes
			err = scannedAttrs.Scan(dbValue)
			assert.NoError(t, err)

			// Convert back to map[string]any for comparison
			result := map[string]any(scannedAttrs)
			assert.Equal(t, tt.expected, result)

			// Verify the duckdb.Map structure matches expected
			for key, expectedValue := range tt.expected {
				union, exists := dbMap[key]
				assert.True(t, exists, "key %s should exist in duckdb.Map", key)

				// Verify the union structure
				duckUnion, ok := union.(duckdb.Union)
				assert.True(t, ok, "value for key %s should be duckdb.Union", key)
				assert.Equal(t, expectedValue, duckUnion.Value, "value mismatch for key %s", key)

				// Verify the tag is correct
				expectedTag, hasTag := tt.expectedTags[key]
				if hasTag {
					assert.Equal(t, expectedTag, duckUnion.Tag, "tag mismatch for key %s", key)
				}
			}
		})
	}
}
