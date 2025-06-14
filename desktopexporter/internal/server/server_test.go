package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func setupEmpty() (*httptest.Server, func()) {
	// Set environment variable to enable logs endpoints
	os.Setenv("ENABLE_LOGS", "true")
	
	server := NewServer("localhost:8000", "")
	testServer := httptest.NewServer(server.Handler())

	return testServer, func() {
		testServer.Close()
		server.Store.Close()
		// Clean up environment variable
		os.Unsetenv("ENABLE_LOGS")
	}
}

func setupWithData(t *testing.T) (*httptest.Server, func(*testing.T)) {
	// Set environment variable to enable logs endpoints
	os.Setenv("ENABLE_LOGS", "true")
	
	baseTime := time.Now().UnixNano()
	server := NewServer("localhost:8000", "")

	// Add test span
	testSpanData := telemetry.SpanData{
		TraceID:      "1234567890",
		TraceState:   "",
		SpanID:       "12345",
		ParentSpanID: "",
		Name:         "test",
		Kind:         "",
		StartTime:    baseTime,
		EndTime:      baseTime + time.Second.Nanoseconds(),
		Attributes:   map[string]any{},
		Events:       []telemetry.EventData{},
		Links:        []telemetry.LinkData{},
		Resource: &telemetry.ResourceData{
			Attributes: map[string]any{
				"service.name": "pumpkin.pie",
			},
			DroppedAttributesCount: 0,
		},
		Scope: &telemetry.ScopeData{
			Name:                   "test.scope",
			Version:                "1",
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
		DroppedAttributesCount: 0,
		DroppedEventsCount:     0,
		DroppedLinksCount:      0,
		StatusCode:             "",
		StatusMessage:          "",
	}

	err := server.Store.AddSpans(context.Background(), []telemetry.SpanData{testSpanData})
	assert.Nilf(t, err, "could not create test span: %v", err)

	// Add test log
	testLogData := telemetry.LogData{
		Timestamp:         baseTime,
		ObservedTimestamp: baseTime + time.Millisecond.Nanoseconds(),
		TraceID:          "1234567890",
		SpanID:           "12345",
		SeverityText:     "INFO",
		SeverityNumber:   9,
		Body:             "test log message",
		Resource: &telemetry.ResourceData{
			Attributes: map[string]any{
				"service.name": "pumpkin.pie",
			},
			DroppedAttributesCount: 0,
		},
		Scope: &telemetry.ScopeData{
			Name:                   "test.scope",
			Version:                "1",
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
		Attributes:             map[string]any{},
		DroppedAttributesCount: 0,
		Flags:                  1,
		EventName:             "test.event",
	}

	err = server.Store.AddLogs(context.Background(), []telemetry.LogData{testLogData})
	assert.Nilf(t, err, "could not create test log: %v", err)

	testServer := httptest.NewServer(server.Handler())

	return testServer, func(t *testing.T) {
		testServer.Close()
		server.Store.Close()
		// Clean up environment variable
		os.Unsetenv("ENABLE_LOGS")
	}
}

func TestTracesHandler(t *testing.T) {
	t.Run("Traces Handler (Empty)", func(t *testing.T) {
		testServer, teardown := setupEmpty()
		defer teardown()

		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces"))
		assert.Nilf(t, err, "could not send GET request %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		// Init summaries struct with some data to be overwritten
		baseTime := time.Now().UnixNano()
		testSummaries := telemetry.TraceSummaries{
			TraceSummaries: []telemetry.TraceSummary{
				{
					TraceID: "12345",
					RootSpan: &telemetry.RootSpan{
						ServiceName: "groot",
						Name:        "i.am.groot",
						StartTime:   baseTime,
						EndTime:     baseTime + time.Minute.Nanoseconds(),
					},
					SpanCount: 2,
				},
			},
		}
		err = json.Unmarshal(b, &testSummaries)
		assert.Nilf(t, err, "could not unmarshal bytes to trace summaries: %v", err)

		assert.Len(t, testSummaries.TraceSummaries, 0)
	})

	t.Run("Traces Handler (Not Empty)", func(t *testing.T) {
		testServer, teardown := setupWithData(t)
		defer teardown(t)

		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testSummaries := telemetry.TraceSummaries{}
		err = json.Unmarshal(b, &testSummaries)
		assert.Nilf(t, err, "could not unmarshal bytes to trace summaries: %v", err)

		assert.Equal(t, "1234567890", testSummaries.TraceSummaries[0].TraceID)
		assert.Equal(t, "test", testSummaries.TraceSummaries[0].RootSpan.Name)
		assert.Equal(t, "pumpkin.pie", testSummaries.TraceSummaries[0].RootSpan.ServiceName)
		assert.Equal(t, uint32(1), testSummaries.TraceSummaries[0].SpanCount)
	})
}

func TestTraceIDHandler(t *testing.T) {
	testServer, teardown := setupWithData(t)
	defer teardown(t)

	t.Run("Trace ID Handler (Not Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces/987654321"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Traces ID Handler (ID Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces/1234567890"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testTrace := telemetry.TraceData{}
		err = json.Unmarshal(b, &testTrace)
		assert.Nilf(t, err, "could not unmarshal bytes to trace data: %v", err)

		assert.Equal(t, "1234567890", testTrace.TraceID)
		assert.Equal(t, "12345", testTrace.Spans[0].SpanID)
		assert.Equal(t, "test", testTrace.Spans[0].Name)
		assert.Equal(t, "pumpkin.pie", testTrace.Spans[0].Resource.Attributes["service.name"])
		assert.Equal(t, 1, len(testTrace.Spans))
	})
}

func TestLogsHandler(t *testing.T) {
	t.Run("Logs Handler (Empty)", func(t *testing.T) {
		testServer, teardown := setupEmpty()
		defer teardown()

		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 0)
	})

	t.Run("Logs Handler (Not Empty)", func(t *testing.T) {
		testServer, teardown := setupWithData(t)
		defer teardown(t)

		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 1)
		assert.Equal(t, "1234567890", testLogs.Logs[0].TraceID)
		assert.Equal(t, "12345", testLogs.Logs[0].SpanID)
		assert.Equal(t, "INFO", testLogs.Logs[0].SeverityText)
		assert.Equal(t, "test log message", testLogs.Logs[0].Body)
		assert.Equal(t, "pumpkin.pie", testLogs.Logs[0].Resource.Attributes["service.name"])
	})
}

func TestLogsByTraceHandler(t *testing.T) {
	testServer, teardown := setupWithData(t)
	defer teardown(t)

	t.Run("Logs By Trace Handler (Not Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs/trace/987654321"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 0)
	})

	t.Run("Logs By Trace Handler (Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs/trace/1234567890"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 1)
		assert.Equal(t, "1234567890", testLogs.Logs[0].TraceID)
		assert.Equal(t, "12345", testLogs.Logs[0].SpanID)
		assert.Equal(t, "INFO", testLogs.Logs[0].SeverityText)
		assert.Equal(t, "test log message", testLogs.Logs[0].Body)
	})
}

func TestClearHandlers(t *testing.T) {
	testServer, teardown := setupWithData(t)
	defer teardown(t)

	t.Run("Clear Traces", func(t *testing.T) {
		// Clear traces
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/clearTraces"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		// Verify traces are cleared
		res, err = http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testSummaries := telemetry.TraceSummaries{}
		err = json.Unmarshal(b, &testSummaries)
		assert.Nilf(t, err, "could not unmarshal bytes to trace summaries: %v", err)

		assert.Len(t, testSummaries.TraceSummaries, 0)

		// Verify logs still exist
		res, err = http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		b, err = io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 1)
	})

	t.Run("Clear Logs", func(t *testing.T) {
		// Clear logs
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/clearLogs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		// Verify logs are cleared
		res, err = http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Len(t, testLogs.Logs, 0)
	})
}

func TestSampleHandler(t *testing.T) {
	testServer, teardown := setupEmpty()
	defer teardown()

	// Populate sample data
	res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/sampleData"))
	assert.Nilf(t, err, "could not send GET request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	t.Run("Sample Data Handler (Traces)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces/42957c7c2fca940a0d32a0cdd38c06a4"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testTrace := telemetry.TraceData{}
		err = json.Unmarshal(b, &testTrace)
		assert.Nilf(t, err, "could not unmarshal bytes to trace data: %v", err)

		assert.Equal(t, "42957c7c2fca940a0d32a0cdd38c06a4", testTrace.TraceID)
		assert.Equal(t, "37fd1349bf83d330", testTrace.Spans[0].SpanID)
		assert.Equal(t, "SAMPLE HTTP POST", testTrace.Spans[0].Name)
		assert.Equal(t, "sample-loadgenerator", testTrace.Spans[0].Resource.Attributes["service.name"])
		assert.Equal(t, 3, len(testTrace.Spans))
	})

	t.Run("Sample Data Handler (Logs)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/logs"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		b, err := io.ReadAll(res.Body)
		assert.Nilf(t, err, "could not read response body: %v", err)

		testLogs := telemetry.Logs{}
		err = json.Unmarshal(b, &testLogs)
		assert.Nilf(t, err, "could not unmarshal bytes to logs: %v", err)

		assert.Greater(t, len(testLogs.Logs), 0, "should have sample logs")
	})
}
