package telemetry_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

var spans []telemetry.SpanData

func init() {
	spans = telemetry.NewSampleTelemetry().Spans
}

func TestSpanExtraction(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, spans []telemetry.SpanData)
	}{
		{
			name: "extracts correct number of spans",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				assert.Len(t, spans, 4)
			},
		},
		{
			name: "validates resource attributes",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				resource := spans[0].Resource
				assert.Equal(t, uint32(0), resource.DroppedAttributesCount)
				
				expectedAttrs := map[string]any{
					"service.name":            "sample.currencyservice",
					"telemetry.sdk.language":  "cpp",
					"telemetry.sdk.name":      "opentelemetry",
					"telemetry.sdk.version":   "1.5.0",
					"array.example":           []any{"example1", "example2", "example3"},
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, resource.Attributes[key], "resource attribute %s", key)
				}
			},
		},
		{
			name: "validates scope attributes",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				scope := spans[0].Scope
				assert.Equal(t, "sample.currencyservice", scope.Name)
				assert.Equal(t, "v1.2.3", scope.Version)
				assert.Equal(t, uint32(2), scope.DroppedAttributesCount)
				
				expectedAttrs := map[string]any{
					"owner.name":    "Mila Ardath",
					"owner.contact": "github.com/CtrlSpice",
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, scope.Attributes[key], "scope attribute %s", key)
				}
			},
		},
		{
			name: "validates event attributes",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				event := spans[0].Events[1]
				assert.Equal(t, "Conversion successful. Response sent back.", event.Name)
				assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179479924, time.UTC), time.Unix(0, event.Timestamp).In(time.UTC))
				assert.Equal(t, uint32(1), event.DroppedAttributesCount)
				
				expectedAttrs := map[string]any{
					"event.class":    "sample",
					"event.priority": int64(1),
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, event.Attributes[key], "event attribute %s", key)
				}
			},
		},
		{
			name: "validates link attributes",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				link := spans[2].Links[0]
				assert.Equal(t, "2c1ae93af4d3f887", link.SpanID)
				assert.Equal(t, "7979cec4d1c04222fa9a3c7c97c0a99c", link.TraceID)
				assert.Equal(t, uint32(5), link.DroppedAttributesCount)
				assert.Equal(t, "in-cart currency conversion", link.Attributes["relationship"])
			},
		},
		{
			name: "validates span attributes",
			validate: func(t *testing.T, spans []telemetry.SpanData) {
				span := spans[3]
				assert.Equal(t, uint32(0), span.DroppedAttributesCount)
				assert.Equal(t, uint32(0), span.DroppedEventsCount)
				assert.Equal(t, uint32(0), span.DroppedLinksCount)
				assert.Equal(t, "SAMPLE HTTP POST", span.Name)
				assert.Equal(t, "Server", span.Kind)
				assert.Equal(t, time.Date(2023, 02, 02, 18, 17, 54, 805039872, time.UTC), time.Unix(0, span.StartTime).In(time.UTC))
				assert.Equal(t, time.Date(2023, 02, 02, 18, 17, 54, 816274688, time.UTC), time.Unix(0, span.EndTime).In(time.UTC))
				assert.Equal(t, "Unset", span.StatusCode)
				assert.Equal(t, "355dc9bea1ec64d8", span.SpanID)
				assert.Equal(t, "a24ac1588d52a6fc", span.ParentSpanID)
				assert.Equal(t, "42957c7c2fca940a0d32a0cdd38c06a4", span.TraceID)
				
				expectedAttrs := map[string]any{
					"http.flavor":        "1.1",
					"http.host":          "frontend:8080",
					"http.method":        "POST",
					"http.request_length": int64(102),
					"http.status_code":   int64(200),
					"http.status_text":   "Ok",
					"http.target":        "/api/cart",
					"http.url":           "http://frontend:8080/api/cart",
					"http.user_agent":    "python-requests/2.27.1",
					"net.host.ip":        "::ffff:172.24.0.22",
					"net.host.name":      "frontend",
					"net.host.port":      int64(8080),
					"net.peer.ip":        "::ffff:172.24.0.23",
					"net.peer.port":      int64(46054),
					"net.transport":      "ip_tcp",
					"array.example":      []any{1.1, 1.2, 1.3},
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, span.Attributes[key], "span attribute %s", key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, spans)
		})
	}
}

func TestSpanMarshaling(t *testing.T) {
	span := spans[0]
	
	jsonBytes, err := span.MarshalJSON()
	assert.NoError(t, err)
	
	var result map[string]any
	err = json.Unmarshal(jsonBytes, &result)
	assert.NoError(t, err)
	
	tests := []struct {
		name     string
		validate func(t *testing.T, result map[string]any)
	}{
		{
			name: "validates timestamp format",
			validate: func(t *testing.T, result map[string]any) {
				// Validate start time
				startTime := result["startTime"].(map[string]any)
				assert.Contains(t, startTime, "milliseconds")
				assert.Contains(t, startTime, "nanoseconds")
				
				// Convert original timestamp to expected format
				startTimeMs := span.StartTime / int64(time.Millisecond)
				startTimeNs := span.StartTime % int64(time.Millisecond)
				assert.Equal(t, float64(startTimeMs), startTime["milliseconds"], "start time milliseconds")
				assert.Equal(t, float64(startTimeNs), startTime["nanoseconds"], "start time nanoseconds")
				
				// Validate end time
				endTime := result["endTime"].(map[string]any)
				assert.Contains(t, endTime, "milliseconds")
				assert.Contains(t, endTime, "nanoseconds")
				
				// Convert original timestamp to expected format
				endTimeMs := span.EndTime / int64(time.Millisecond)
				endTimeNs := span.EndTime % int64(time.Millisecond)
				assert.Equal(t, float64(endTimeMs), endTime["milliseconds"], "end time milliseconds")
				assert.Equal(t, float64(endTimeNs), endTime["nanoseconds"], "end time nanoseconds")
			},
		},
		{
			name: "validates basic fields",
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, span.TraceID, result["traceID"])
				assert.Equal(t, span.SpanID, result["spanID"])
				assert.Equal(t, span.Name, result["name"])
				assert.Equal(t, span.Kind, result["kind"])
			},
		},
		{
			name: "validates events",
			validate: func(t *testing.T, result map[string]any) {
				events := result["events"].([]any)
				assert.Len(t, events, len(span.Events))
				
				// Validate first event timestamp
				firstEvent := events[0].(map[string]any)
				eventTimestamp := firstEvent["timestamp"].(map[string]any)
				assert.Contains(t, eventTimestamp, "milliseconds")
				assert.Contains(t, eventTimestamp, "nanoseconds")
				
				// Convert original event timestamp to expected format
				eventTimeMs := span.Events[0].Timestamp / int64(time.Millisecond)
				eventTimeNs := span.Events[0].Timestamp % int64(time.Millisecond)
				assert.Equal(t, float64(eventTimeMs), eventTimestamp["milliseconds"], "event time milliseconds")
				assert.Equal(t, float64(eventTimeNs), eventTimestamp["nanoseconds"], "event time nanoseconds")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, result)
		})
	}
}
