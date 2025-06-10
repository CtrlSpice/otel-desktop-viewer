package telemetry_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

var logs []telemetry.LogData
var logIDs map[string]*telemetry.LogData

func init() {
	logs = telemetry.NewSampleTelemetry().Logs
	// Create a map of log IDs for easy lookup
	logIDs = make(map[string]*telemetry.LogData)
	for i := range logs {
		logIDs[logs[i].ID()] = &logs[i]
	}
}

func TestLogExtraction(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, logs []telemetry.LogData)
	}{
		{
			name: "extracts correct number of logs",
			validate: func(t *testing.T, logs []telemetry.LogData) {
				assert.Len(t, logs, 3)
			},
		},
		{
			name: "validates resource attributes",
			validate: func(t *testing.T, logs []telemetry.LogData) {
				// Find currency service log by its ID
				currencyLog := logIDs[logs[0].ID()]
				assert.NotNil(t, currencyLog, "should find currency log")
				
				resource := currencyLog.Resource
				assert.Equal(t, uint32(0), resource.DroppedAttributesCount)
				
				expectedAttrs := map[string]any{
					"service.name":            "sample.currencyservice",
					"telemetry.sdk.language":  "cpp",
					"telemetry.sdk.name":      "opentelemetry",
					"telemetry.sdk.version":   "1.5.0",
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, resource.Attributes[key], "resource attribute %s", key)
				}
			},
		},
		{
			name: "validates scope attributes",
			validate: func(t *testing.T, logs []telemetry.LogData) {
				// Find currency service log by its ID
				currencyLog := logIDs[logs[0].ID()]
				assert.NotNil(t, currencyLog, "should find currency log")
				
				scope := currencyLog.Scope
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
			name: "validates log attributes",
			validate: func(t *testing.T, logs []telemetry.LogData) {
				// Find currency service log by its ID
				currencyLog := logIDs[logs[0].ID()]
				assert.NotNil(t, currencyLog, "should find currency log")
				
				assert.Equal(t, uint32(0), currencyLog.DroppedAttributesCount)
				assert.Equal(t, "ERROR", currencyLog.SeverityText)
				assert.Equal(t, int32(17), currencyLog.SeverityNumber)
				assert.Equal(t, "currency.conversion.failed", currencyLog.EventName)
				assert.Equal(t, uint32(0), currencyLog.Flags)
				
				expectedAttrs := map[string]any{
					"currency.from":   "USD",
					"currency.to":     "CAD",
					"currency.amount": float64(100.50),
				}
				
				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, currencyLog.Attributes[key], "log attribute %s", key)
				}
			},
		},
		{
			name: "validates log bodies",
			validate: func(t *testing.T, logs []telemetry.LogData) {
				// Find logs by their IDs
				currencyLog := logIDs[logs[0].ID()]
				httpLog := logIDs[logs[1].ID()]
				systemLog := logIDs[logs[2].ID()]
				
				assert.NotNil(t, currencyLog, "should find currency log")
				assert.NotNil(t, httpLog, "should find http log")
				assert.NotNil(t, systemLog, "should find system log")
				
				// Validate currency log body
				assert.Equal(t, "Currency conversion failed: invalid amount", currencyLog.Body)

				// Validate HTTP log body
				assert.Equal(t, "HTTP request completed", httpLog.Body)

				// Validate system log body
				assert.Equal(t, "High memory usage detected", systemLog.Body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, logs)
		})
	}
}

func TestLogMarshaling(t *testing.T) {
	// Find currency log by its ID
	currencyLog := logIDs[logs[0].ID()]
	assert.NotNil(t, currencyLog, "should find currency log")
	
	jsonBytes, err := currencyLog.MarshalJSON()
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
				// Validate timestamp
				timestamp := result["timestamp"].(map[string]any)
				assert.Contains(t, timestamp, "milliseconds")
				assert.Contains(t, timestamp, "nanoseconds")
				
				// Convert original timestamp to expected format
				timestampMs := currencyLog.Timestamp / int64(time.Millisecond)
				timestampNs := currencyLog.Timestamp % int64(time.Millisecond)
				assert.Equal(t, float64(timestampMs), timestamp["milliseconds"], "timestamp milliseconds")
				assert.Equal(t, float64(timestampNs), timestamp["nanoseconds"], "timestamp nanoseconds")

				// Validate observed timestamp
				observedTimestamp := result["observedTimestamp"].(map[string]any)
				assert.Contains(t, observedTimestamp, "milliseconds")
				assert.Contains(t, observedTimestamp, "nanoseconds")
				
				observedMs := currencyLog.ObservedTimestamp / int64(time.Millisecond)
				observedNs := currencyLog.ObservedTimestamp % int64(time.Millisecond)
				assert.Equal(t, float64(observedMs), observedTimestamp["milliseconds"], "observed timestamp milliseconds")
				assert.Equal(t, float64(observedNs), observedTimestamp["nanoseconds"], "observed timestamp nanoseconds")
			},
		},
		{
			name: "validates basic fields",
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, currencyLog.TraceID, result["traceID"])
				assert.Equal(t, currencyLog.SpanID, result["spanID"])
				assert.Equal(t, currencyLog.SeverityText, result["severityText"])
				assert.Equal(t, float64(currencyLog.SeverityNumber), result["severityNumber"])
				assert.Equal(t, currencyLog.EventName, result["eventName"])
				// Flags is omitted when zero, so we should check if it exists first
				if flags, ok := result["flags"]; ok {
					assert.Equal(t, float64(currencyLog.Flags), flags)
				} else {
					assert.Equal(t, uint32(0), currencyLog.Flags, "flags should be zero when omitted")
				}
			},
		},
		{
			name: "validates body",
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "Currency conversion failed: invalid amount", result["body"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, result)
		})
	}
} 