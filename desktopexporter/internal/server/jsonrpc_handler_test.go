package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/jsonrpc2"
)

func setupHandler() (*JSONRPCHandler, func()) {
	store := store.NewStore(context.Background(), "")
	handler := NewJSONRPCHandler(store)
	return handler, func() {
		store.Close()
	}
}

func setupHandlerWithData(t *testing.T) (*JSONRPCHandler, func()) {
	store := store.NewStore(context.Background(), "")
	handler := NewJSONRPCHandler(store)

	// Add test span
	baseTime := time.Now().UnixNano()
	testSpanData := traces.SpanData{
		TraceID:      "1234567890",
		TraceState:   "",
		SpanID:       "12345",
		ParentSpanID: "",
		Name:         "test",
		Kind:         "",
		StartTime:    baseTime,
		EndTime:      baseTime + time.Second.Nanoseconds(),
		Attributes:   map[string]any{},
		Events:       []traces.EventData{},
		Links:        []traces.LinkData{},
		Resource: &resource.ResourceData{
			Attributes: map[string]any{
				"service.name": "pumpkin.pie",
			},
			DroppedAttributesCount: 0,
		},
		Scope: &scope.ScopeData{
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

	err := handler.store.AddSpans(context.Background(), []traces.SpanData{testSpanData})
	assert.Nilf(t, err, "could not create test span: %v", err)

	// Add test log
	testLogData := logs.LogData{
		Timestamp:         baseTime,
		ObservedTimestamp: baseTime + time.Millisecond.Nanoseconds(),
		TraceID:           "1234567890",
		SpanID:            "12345",
		SeverityText:      "INFO",
		SeverityNumber:    9,
		Body:              logs.Body{Data: "test log message"},
		Resource: &resource.ResourceData{
			Attributes: map[string]any{
				"service.name": "pumpkin.pie",
			},
			DroppedAttributesCount: 0,
		},
		Scope: &scope.ScopeData{
			Name:                   "test.scope",
			Version:                "1",
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
		Attributes:             map[string]any{},
		DroppedAttributesCount: 0,
		Flags:                  1,
		EventName:              "test.event",
	}

	err = handler.store.AddLogs(context.Background(), []logs.LogData{testLogData})
	assert.Nilf(t, err, "could not create test log: %v", err)

	return handler, func() {
		store.Close()
	}
}

func createRequest(method string, params any) *jsonrpc2.Request {
	paramsBytes, _ := json.Marshal(params)
	return &jsonrpc2.Request{
		Method: method,
		Params: paramsBytes,
		ID:     jsonrpc2.Int64ID(1),
	}
}

func TestGetTraceSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler()
		defer teardown()

		req := createRequest("getTraceSummaries", nil)
		result, err := handler.Handle(context.Background(), req)

		assert.Nil(t, err)
		summaries, ok := result.([]traces.TraceSummary)
		assert.True(t, ok, "Expected []traces.TraceSummary, got %T", result)
		assert.Len(t, summaries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		req := createRequest("getTraceSummaries", nil)
		result, err := handler.Handle(context.Background(), req)

		assert.Nil(t, err)
		summaries, ok := result.([]traces.TraceSummary)
		assert.True(t, ok, "Expected []traces.TraceSummary, got %T", result)
		assert.Len(t, summaries, 1)
		assert.Equal(t, "1234567890", summaries[0].TraceID)
	})
}

func TestGetTraceByID(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	t.Run("Found", func(t *testing.T) {
		req := createRequest("getTraceByID", []string{"1234567890"})
		result, err := handler.Handle(context.Background(), req)

		assert.Nil(t, err)
		trace, ok := result.(traces.TraceData)
		assert.True(t, ok)
		assert.Equal(t, "1234567890", trace.TraceID)
		assert.Len(t, trace.Spans, 1)
	})

	t.Run("Not Found", func(t *testing.T) {
		req := createRequest("getTraceByID", []string{"nonexistent"})
		result, err := handler.Handle(context.Background(), req)

		assert.NotNil(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrTraceNotFound, err)
	})
}

func TestClearTraces(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	req := createRequest("clearTraces", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.Nil(t, err)
	assert.Equal(t, "Traces cleared successfully", result)

	// Verify traces are cleared
	getReq := createRequest("getTraceSummaries", nil)
	getResult, getErr := handler.Handle(context.Background(), getReq)
	assert.Nil(t, getErr)
	summaries, ok := getResult.([]traces.TraceSummary)
	assert.True(t, ok, "Expected []traces.TraceSummary, got %T", getResult)
	assert.Len(t, summaries, 0)
}

func TestGetLogs(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler()
		defer teardown()

		req := createRequest("getLogs", nil)
		result, err := handler.Handle(context.Background(), req)

		assert.Nil(t, err)
		logsData, ok := result.([]logs.LogData)
		assert.True(t, ok, "Expected []logs.LogData, got %T", result)
		assert.Len(t, logsData, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		req := createRequest("getLogs", nil)
		result, err := handler.Handle(context.Background(), req)

		assert.Nil(t, err)
		logsData, ok := result.([]logs.LogData)
		assert.True(t, ok, "Expected []logs.LogData, got %T", result)
		assert.Len(t, logsData, 1)
		assert.Equal(t, "1234567890", logsData[0].TraceID)
		assert.Equal(t, "test log message", logsData[0].Body.Data)
	})
}

func TestMethodNotFound(t *testing.T) {
	handler, teardown := setupHandler()
	defer teardown()

	req := createRequest("nonexistentMethod", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.NotNil(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrMethodNotFound, err)
}
