package store

import (
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestEscapeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "handles single quotes",
			input:    "it's working",
			expected: "it''s working",
		},
		{
			name:     "handles multiple single quotes",
			input:    "it's really 'working' now",
			expected: "it''s really ''working'' now",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "handles string without quotes",
			input:    "normal string",
			expected: "normal string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapToString(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name: "handles string values",
			input: map[string]interface{}{
				"key": "value",
			},
			expected: "MAP{'key': 'value'::attribute}",
		},
		{
			name: "handles integer values",
			input: map[string]interface{}{
				"count": 42,
			},
			expected: "MAP{'count': 42::attribute}",
		},
		{
			name: "handles boolean values",
			input: map[string]interface{}{
				"enabled": true,
			},
			expected: "MAP{'enabled': true::attribute}",
		},
		{
			name:     "handles empty map",
			input:    map[string]interface{}{},
			expected: "MAP{}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapToStringMultipleValues(t *testing.T) {
	input := map[string]interface{}{
		"name":    "test",
		"count":   42,
		"enabled": true,
	}
	result := mapToString(input)

	// Check that the result starts and ends correctly
	assert.Contains(t, result, "MAP{")
	assert.Contains(t, result, "}")

	// Check that all key-value pairs are present
	assert.Contains(t, result, "'name': 'test'::attribute")
	assert.Contains(t, result, "'count': 42::attribute")
	assert.Contains(t, result, "'enabled': true::attribute")
}

func TestEventToString(t *testing.T) {
	timestamp := time.Now()
	tests := []struct {
		name     string
		input    []telemetry.EventData
		expected string
	}{
		{
			name: "handles single event",
			input: []telemetry.EventData{
				{
					Name:                   "test event",
					Timestamp:              timestamp,
					Attributes:             map[string]interface{}{"key": "value"},
					DroppedAttributesCount: 0,
				},
			},
			expected: "[{name: 'test event', timestamp: '" + timestamp.Format(time.RFC3339Nano) + "', attributes: MAP{'key': 'value'::attribute}, droppedAttributesCount: 0}]",
		},
		{
			name:     "handles empty events",
			input:    []telemetry.EventData{},
			expected: "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLinkToString(t *testing.T) {
	tests := []struct {
		name     string
		input    []telemetry.LinkData
		expected string
	}{
		{
			name: "handles single link",
			input: []telemetry.LinkData{
				{
					TraceID:                "trace1",
					SpanID:                 "span1",
					TraceState:             "state1",
					Attributes:             map[string]interface{}{"key": "value"},
					DroppedAttributesCount: 0,
				},
			},
			expected: "[{traceID: 'trace1', spanID: 'span1', traceState: 'state1', attributes: MAP{'key': 'value'::attribute}, droppedAttributesCount: 0}]",
		},
		{
			name:     "handles empty links",
			input:    []telemetry.LinkData{},
			expected: "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := linkToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRawAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]interface{}
	}{
		{
			name:  "parses string value",
			input: "{key='value'}",
			expected: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name:  "parses number value",
			input: "{count=42}",
			expected: map[string]interface{}{
				"count": int64(42),
			},
		},
		{
			name:  "parses boolean value",
			input: "{enabled=true}",
			expected: map[string]interface{}{
				"enabled": true,
			},
		},
		{
			name:  "parses string array",
			input: "{tags=['tag1' 'tag 2' 'tag3']}",
			expected: map[string]interface{}{
				"tags": []string{"tag1", "tag 2", "tag3"},
			},
		},
		{
			name:  "parses number array",
			input: "{counts=[1 2 3]}",
			expected: map[string]interface{}{
				"counts": []int64{1, 2, 3},
			},
		},
		{
			name:  "parses boolean array",
			input: "{flags=[true false true]}",
			expected: map[string]interface{}{
				"flags": []bool{true, false, true},
			},
		},
		{
			name:  "parses empty arrays",
			input: "{empty_list=[]}",
			expected: map[string]interface{}{
				"empty_list": []interface{}{},
			},
		},
		{
			name:     "parses empty map",
			input:    "{}",
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRawAttributes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
