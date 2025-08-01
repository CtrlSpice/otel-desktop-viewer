package logs

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var logs []LogData
var logIDs map[string]*LogData

func init() {
	logs = GenerateSampleLogs()
	// Create a map of log IDs for easy lookup
	logIDs = make(map[string]*LogData)
	for i := range logs {
		logIDs[logs[i].ID()] = &logs[i]
	}
}

func TestLogExtraction(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, logs []LogData)
	}{
		{
			name: "extracts correct number of logs",
			validate: func(t *testing.T, logs []LogData) {
				assert.Len(t, logs, 3)
			},
		},
		{
			name: "validates log attributes",
			validate: func(t *testing.T, logs []LogData) {
				// Find currency service log by its ID
				currencyLog := logIDs[logs[0].ID()]
				assert.NotNil(t, currencyLog, "should find currency log")

				assert.Equal(t, uint32(0), currencyLog.DroppedAttributesCount)
				assert.Equal(t, "ERROR", currencyLog.SeverityText)
				assert.Equal(t, int32(17), currencyLog.SeverityNumber)
				assert.Equal(t, "currency.conversion.failed", currencyLog.EventName)
				assert.Equal(t, uint32(1), currencyLog.Flags)

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
			validate: func(t *testing.T, logs []LogData) {
				// Find logs by their IDs
				currencyLog := logIDs[logs[0].ID()]
				httpLog := logIDs[logs[1].ID()]
				systemLog := logIDs[logs[2].ID()]

				assert.NotNil(t, currencyLog, "should find currency log")
				assert.NotNil(t, httpLog, "should find http log")
				assert.NotNil(t, systemLog, "should find system log")

				// Validate currency log body
				assert.Equal(t, "Currency conversion failed: invalid amount", currencyLog.Body.Data)

				// Validate HTTP log body
				assert.Equal(t, "HTTP request completed", httpLog.Body.Data)

				// Validate system log body
				assert.Equal(t, "High memory usage detected", systemLog.Body.Data)
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
				// Validate timestamp is serialized as string
				timestamp, ok := result["timestamp"].(string)
				assert.True(t, ok, "timestamp should be a string")
				assert.NotEmpty(t, timestamp, "timestamp should not be empty")

				// Validate observed timestamp is serialized as string
				observedTimestamp, ok := result["observedTimestamp"].(string)
				assert.True(t, ok, "observedTimestamp should be a string")
				assert.NotEmpty(t, observedTimestamp, "observedTimestamp should not be empty")

				// Verify the strings contain valid numeric values
				_, err := strconv.ParseInt(timestamp, 10, 64)
				assert.NoError(t, err, "timestamp should be a valid integer string")

				_, err = strconv.ParseInt(observedTimestamp, 10, 64)
				assert.NoError(t, err, "observedTimestamp should be a valid integer string")
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
