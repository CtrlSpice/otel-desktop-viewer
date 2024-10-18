package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func setupEmpty() (*httptest.Server, func()) {
	server := NewServer("localhost:8000")
	testServer := httptest.NewServer(server.Handler(false))

	return testServer, func() {
		testServer.Close()
		server.Store.Close()
	}
}

func setupWithTrace(t *testing.T) (*httptest.Server, func(*testing.T)) {
	server := NewServer("localhost:8000")
	testSpanData := telemetry.SpanData{
		TraceID:      "1234567890",
		TraceState:   "",
		SpanID:       "12345",
		ParentSpanID: "",
		Name:         "test",
		Kind:         "",
		StartTime:    time.Now(),
		EndTime:      time.Now().Add(time.Second),
		Attributes:   map[string]interface{}{},
		Events:       []telemetry.EventData{},
		Links:        []telemetry.LinkData{},
		Resource:     &telemetry.ResourceData{Attributes: map[string]any{"service.name": "pumpkin.pie"}, DroppedAttributesCount: 0},
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
	assert.Nilf(t, err, "could not create  test span: %v", err)

	testServer := httptest.NewServer(server.Handler(false))

	return testServer, func(t *testing.T) {
		testServer.Close()
		server.Store.Close()
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
		testSummaries := telemetry.TraceSummaries{
			TraceSummaries: []telemetry.TraceSummary{
				{
					HasRootSpan:     true,
					RootServiceName: "groot",
					RootName:        "i.am.groot",
					RootStartTime:   time.Now(),
					RootEndTime:     time.Now().Add(time.Minute),
					SpanCount:       2,
					TraceID:         "12345",
				},
			},
		}
		err = json.Unmarshal(b, &testSummaries)
		assert.Nilf(t, err, "could not unmarshal bytes to trace summaries: %v", err)

		assert.Len(t, testSummaries.TraceSummaries, 0)
	})

	t.Run("Traces Handler (Not Empty)", func(t *testing.T) {
		testServer, teardown := setupWithTrace(t)
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
		assert.Equal(t, true, testSummaries.TraceSummaries[0].HasRootSpan)
		assert.Equal(t, "test", testSummaries.TraceSummaries[0].RootName)
		assert.Equal(t, "pumpkin.pie", testSummaries.TraceSummaries[0].RootServiceName)
		assert.Equal(t, uint32(1), testSummaries.TraceSummaries[0].SpanCount)
	})
}

func TestTraceIDHandler(t *testing.T) {
	testServer, teardown := setupWithTrace(t)
	defer teardown(t)

	t.Run("Trace ID Handler (Not Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces/987654321"))
		assert.Nilf(t, err, "could not send GET request: %v", err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
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

func TestClearTracesHandler(t *testing.T) {
	testServer, teardown := setupWithTrace(t)
	defer teardown(t)

	// Clear dat data
	res, err := http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/clearData"))
	assert.Nilf(t, err, "could not send GET request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Get trace summaries
	res, err = http.Get(fmt.Sprintf("%s%s", testServer.URL, "/api/traces"))
	assert.Nilf(t, err, "could not send GET request: %v", err)

	assert.Equal(t, http.StatusOK, res.StatusCode)

	b, err := io.ReadAll(res.Body)
	assert.Nilf(t, err, "could not read response body: %v", err)

	testSummaries := telemetry.TraceSummaries{}
	err = json.Unmarshal(b, &testSummaries)
	assert.Nilf(t, err, "could not unmarshal bytes to trace summaries: %v", err)

	// Check that there are no traces in store
	assert.Len(t, testSummaries.TraceSummaries, 0)
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
}
