package store

// import (
// 	"testing"
// 	"time"

// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
// 	"github.com/marcboeker/go-duckdb/v2"
// 	"github.com/stretchr/testify/assert"
// )

// func TestParseRawAttributes(t *testing.T) {
// 	tests := []struct {
// 		name           string
// 		rawAttributes  map[string]duckdb.Union
// 		expectedResult map[string]any
// 	}{
// 		{
// 			name:           "nil attributes",
// 			rawAttributes:  nil,
// 			expectedResult: map[string]any{},
// 		},
// 		{
// 			name: "empty attributes",
// 			rawAttributes: map[string]duckdb.Union{},
// 			expectedResult: map[string]any{},
// 		},
// 		{
// 			name: "various attribute types",
// 			rawAttributes: map[string]duckdb.Union{
// 				"string":  {Tag: "str", Value: "hello"},
// 				"int":     {Tag: "bigint", Value: int64(42)},
// 				"float":   {Tag: "double", Value: float64(3.14)},
// 				"bool":    {Tag: "boolean", Value: true},
// 				"strings": {Tag: "str_list", Value: []string{"a", "b", "c"}},
// 				"ints":    {Tag: "bigint_list", Value: []int64{1, 2, 3}},
// 				"floats":  {Tag: "double_list", Value: []float64{1.1, 2.2, 3.3}},
// 				"bools":   {Tag: "boolean_list", Value: []bool{true, false, true}},
// 			},
// 			expectedResult: map[string]any{
// 				"string":  "hello",
// 				"int":     int64(42),
// 				"float":   float64(3.14),
// 				"bool":    true,
// 				"strings": []string{"a", "b", "c"},
// 				"ints":    []int64{1, 2, 3},
// 				"floats":  []float64{1.1, 2.2, 3.3},
// 				"bools":   []bool{true, false, true},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := fromDuckDBMap(tt.rawAttributes)
// 			assert.Equal(t, tt.expectedResult, result)
// 		})
// 	}
// }

// func TestToDuckDBMap(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    map[string]any
// 		expected duckdb.Map
// 	}{
// 		{
// 			name:     "empty map",
// 			input:    map[string]any{},
// 			expected: duckdb.Map{},
// 		},
// 		{
// 			name: "simple attributes",
// 			input: map[string]any{
// 				"string":  "value",
// 				"int":     int64(42),
// 				"float":   float64(3.14),
// 				"bool":    true,
// 				"strings": []string{"a", "b", "c"},
// 			},
// 			expected: duckdb.Map{
// 				"string":  "value",
// 				"int":     int64(42),
// 				"float":   float64(3.14),
// 				"bool":    true,
// 				"strings": []string{"a", "b", "c"},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := toDuckDBMap(tt.input)
// 			assert.Equal(t, tt.expected, result)
// 		})
// 	}
// }

// func TestToDuckDBEvents(t *testing.T) {
// 	now := time.Now()
// 	tests := []struct {
// 		name     string
// 		input    []telemetry.EventData
// 		expected []duckDBEvent
// 	}{
// 		{
// 			name:     "empty events",
// 			input:    []telemetry.EventData{},
// 			expected: []duckDBEvent{},
// 		},
// 		{
// 			name: "single event",
// 			input: []telemetry.EventData{
// 				{
// 					Name:                   "test event",
// 					Timestamp:             now,
// 					Attributes:            map[string]any{"key": "value"},
// 					DroppedAttributesCount: 0,
// 				},
// 			},
// 			expected: []duckDBEvent{
// 				{
// 					Name:                   "test event",
// 					Timestamp:             now,
// 					Attributes:            duckdb.Map{"key": "value"},
// 					DroppedAttributesCount: 0,
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := toDuckDBEvents(tt.input)
// 			assert.Equal(t, tt.expected, result)
// 		})
// 	}
// }

// func TestToDuckDBLinks(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    []telemetry.LinkData
// 		expected []duckDBLink
// 	}{
// 		{
// 			name:     "empty links",
// 			input:    []telemetry.LinkData{},
// 			expected: []duckDBLink{},
// 		},
// 		{
// 			name: "single link",
// 			input: []telemetry.LinkData{
// 				{
// 					TraceID:               "trace1",
// 					SpanID:               "span1",
// 					TraceState:           "state1",
// 					Attributes:           map[string]any{"key": "value"},
// 					DroppedAttributesCount: 0,
// 				},
// 			},
// 			expected: []duckDBLink{
// 				{
// 					TraceID:               "trace1",
// 					SpanID:               "span1",
// 					TraceState:           "state1",
// 					Attributes:           duckdb.Map{"key": "value"},
// 					DroppedAttributesCount: 0,
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := toDuckDBLinks(tt.input)
// 			assert.Equal(t, tt.expected, result)
// 		})
// 	}
// }

// func TestFromDuckDBMap(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    map[string]duckdb.Union
// 		expected map[string]any
// 	}{
// 		{
// 			name:     "empty map",
// 			input:    map[string]duckdb.Union{},
// 			expected: map[string]any{},
// 		},
// 		{
// 			name: "simple attributes",
// 			input: map[string]duckdb.Union{
// 				"string":  {Value: "value"},
// 				"int":     {Value: int64(42)},
// 				"float":   {Value: float64(3.14)},
// 				"bool":    {Value: true},
// 				"strings": {Value: []string{"a", "b", "c"}},
// 			},
// 			expected: map[string]any{
// 				"string":  "value",
// 				"int":     int64(42),
// 				"float":   float64(3.14),
// 				"bool":    true,
// 				"strings": []string{"a", "b", "c"},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := fromDuckDBMap(tt.input)
// 			assert.Equal(t, tt.expected, result)
// 		})
// 	}
// }
