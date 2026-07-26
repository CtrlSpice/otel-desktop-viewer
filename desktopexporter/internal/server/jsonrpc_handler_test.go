package server

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"golang.org/x/exp/jsonrpc2"
)

func setupHandler(t *testing.T) (*JSONRPCHandler, func()) {
	t.Helper()
	s, err := store.NewStore(context.Background(), "")
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s)
	return handler, func() {
		s.Close()
	}
}

// buildTestTraces returns ptrace.Traces with one span (trace ID 00...01) for handler tests.
func buildTestTraces() ptrace.Traces {
	tr := ptrace.NewTraces()
	base := time.Now().UnixNano()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "pumpkin.pie")
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetName("test")
	span.SetStartTimestamp(pcommon.Timestamp(base))
	span.SetEndTimestamp(pcommon.Timestamp(base + time.Second.Nanoseconds()))
	return tr
}

// buildTestLogs returns plog.Logs with one log for handler tests.
func buildTestLogs() plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "pumpkin.pie")
	sl := rl.ScopeLogs().AppendEmpty()
	rec := sl.LogRecords().AppendEmpty()
	rec.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	rec.Body().SetStr("test log message")
	rec.SetSeverityText("INFO")
	rec.SetSeverityNumber(plog.SeverityNumberInfo)
	return logs
}

func setupHandlerWithData(t *testing.T) (*JSONRPCHandler, func()) {
	t.Helper()
	s, err := store.NewStore(context.Background(), "")
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s)
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildTestTraces())
	})
	assert.NoError(t, err, "ingest spans")

	err = s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, buildTestLogs())
	})
	assert.NoError(t, err, "ingest logs")

	return handler, func() {
		s.Close()
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

const testTraceIDHex = "00000000000000000000000000000001"

func TestSearchTraces(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		req := createRequest("searchTraces", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Len(t, summaries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		req := createRequest("searchTraces", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1, "searchTraces should return the ingested trace")
		assert.Equal(t, testTraceIDHex, summaries[0]["traceID"])
	})
}

func TestSearchSpans(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	t.Run("Found", func(t *testing.T) {
		req := createRequest("searchSpans", []string{testTraceIDHex})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var trace map[string]any
		assert.NoError(t, json.Unmarshal(raw, &trace))
		assert.Equal(t, testTraceIDHex, trace["traceID"])
		spans, _ := trace["spans"].([]any)
		assert.Len(t, spans, 1)
	})

	t.Run("Not Found", func(t *testing.T) {
		req := createRequest("searchSpans", []string{"00000000-0000-0000-0000-000000000099"})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrTraceNotFound, err)
	})
}

func TestClearTraces(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	req := createRequest("clearTraces", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, "Traces cleared successfully", result)

	searchReq := createRequest("searchTraces", []string{"0", strconv.FormatInt(1<<63-1, 10)})
	searchResult, searchErr := handler.Handle(context.Background(), searchReq)
	assert.NoError(t, searchErr)
	raw, ok := searchResult.(json.RawMessage)
	assert.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0)
}

func TestSearchLogs(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		req := createRequest("searchLogs", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Len(t, entries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		req := createRequest("searchLogs", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1, "searchLogs should return the ingested log")
		// searchLogs now returns LogSummary (lightweight) with
		// bodyPreview rather than the full body; getLog returns
		// the full LogData on demand. Verify both shapes here.
		assert.Equal(t, "test log message", entries[0]["bodyPreview"])

		logID, ok := entries[0]["id"].(string)
		require.True(t, ok, "summary should carry an id for detail fetch")
		getReq := createRequest("getLog", []string{logID})
		getResult, getErr := handler.Handle(context.Background(), getReq)
		assert.NoError(t, getErr)
		getRaw, ok := getResult.(json.RawMessage)
		assert.True(t, ok)
		var full map[string]any
		assert.NoError(t, json.Unmarshal(getRaw, &full))
		assert.Equal(t, "test log message", full["body"])
	})
}

func TestGetTraceAttributes(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		now := time.Now().UnixNano()
		req := createRequest("getTraceAttributes", []string{strconv.FormatInt(now-24*time.Hour.Nanoseconds(), 10), strconv.FormatInt(now+24*time.Hour.Nanoseconds(), 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		assert.Equal(t, []byte("[]"), []byte(raw), "empty range should return []")
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		now := time.Now().UnixNano()
		req := createRequest("getTraceAttributes", []string{strconv.FormatInt(now-24*time.Hour.Nanoseconds(), 10), strconv.FormatInt(now+24*time.Hour.Nanoseconds(), 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		assert.NotEmpty(t, raw, "Should have discovered attributes")

		var attrs []struct {
			Name           string `json:"name"`
			AttributeScope string `json:"attributeScope"`
			Type           string `json:"type"`
		}
		assert.NoError(t, json.Unmarshal(raw, &attrs))
		found := false
		for _, a := range attrs {
			if a.Name == "service.name" && a.AttributeScope == "resource" {
				found = true
				assert.Equal(t, "string", a.Type)
				break
			}
		}
		assert.True(t, found, "Should have found service.name resource attribute")
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		req := createRequest("getTraceAttributes", []string{"123"})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})

	t.Run("Invalid Parameter Types", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		req := createRequest("getTraceAttributes", []string{"pumpkin", "pie"})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

// TestDeleteParamValidation covers the ID validation shared by the three
// delete methods: bad params must return the signal-specific invalid-ID code
// (or ErrInvalidParams for malformed arrays), never reach SQL and surface as
// an internal error.
func TestDeleteParamValidation(t *testing.T) {
	cases := []struct {
		method     string
		invalidErr error
	}{
		{"deleteSpansByTraceID", ErrInvalidTraceID},
		{"deleteSpanByID", ErrInvalidSpanID},
		{"deleteLogByID", ErrInvalidLogID},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			handler, teardown := setupHandler(t)
			defer teardown()
			ctx := context.Background()

			t.Run("Empty Array", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []string{}))
				assert.Nil(t, result)
				assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
			})

			t.Run("Non-Array Params", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, "not-an-array"))
				assert.Nil(t, result)
				assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
			})

			t.Run("Non-String Element", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []any{42}))
				assert.Nil(t, result)
				assert.Equal(t, tc.invalidErr, err)
			})

			t.Run("Null Element", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []any{nil}))
				assert.Nil(t, result)
				assert.Equal(t, tc.invalidErr, err)
			})

			t.Run("Malformed ID", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []string{"definitely-not-a-uuid"}))
				assert.Nil(t, result)
				assert.Equal(t, tc.invalidErr, err)
			})

			t.Run("One Bad Apple", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []any{testTraceIDHex, "nope"}))
				assert.Nil(t, result)
				assert.Equal(t, tc.invalidErr, err)
			})

			t.Run("Valid Hex And UUID Forms", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []string{
					testTraceIDHex,
					"00000000-0000-0000-0000-000000000001",
				}))
				assert.NoError(t, err)
				response, ok := result.(map[string]any)
				require.True(t, ok, "expected map response, got %T", result)
				assert.Equal(t, 2, response["count"])
			})

			// Span payloads carry 16-char hex span IDs (OTLP wire form);
			// only deleteSpanByID accepts them.
			t.Run("16-Hex Wire Form", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []string{"0000000000000001"}))
				if tc.method == "deleteSpanByID" {
					assert.NoError(t, err)
				} else {
					assert.Nil(t, result)
					assert.Equal(t, tc.invalidErr, err)
				}
			})
		})
	}
}

// TestReadPathIDValidation covers the single-ID validation on read methods:
// a malformed ID returns the signal-specific code instead of reaching SQL
// and surfacing as a cast error dressed up as ErrInternal.
func TestReadPathIDValidation(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()
	ctx := context.Background()

	cases := []struct {
		method     string
		params     any
		invalidErr error
	}{
		{"searchSpans", []string{"not-a-trace-id"}, ErrInvalidTraceID},
		{"getLog", []string{"not-a-log-id"}, ErrInvalidLogID},
		{"getMetric", []string{"not-a-stream-id", "0", "1"}, ErrInvalidStreamID},
		{"getAttributesByTraceID", []string{"not-a-trace-id"}, ErrInvalidTraceID},
		{"getTraceSpanCount", []string{"not-a-trace-id"}, ErrInvalidTraceID},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			result, err := handler.Handle(ctx, createRequest(tc.method, tc.params))
			assert.Nil(t, result)
			assert.Equal(t, tc.invalidErr, err)
		})
	}
}

func TestMethodNotFound(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("nonexistentMethod", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrMethodNotFound, err)
}

// TestSearchLogsInvalidParams ensures searchLogs with wrong param count returns ErrInvalidParams.
func TestSearchLogsInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("searchLogs", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}

// TestSearchMetricSummariesInvalidParams ensures searchMetricSummaries with wrong param count returns ErrInvalidParams.
func TestSearchMetricSummariesInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("searchMetricSummaries", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}


func TestGetStats(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	result, err := handler.Handle(context.Background(), createRequest("getStats", nil))

	assert.NoError(t, err)
	raw, ok := result.(json.RawMessage)
	require.True(t, ok, "Expected json.RawMessage, got %T", result)
	var stats struct {
		Storage struct {
			SizeBytes float64 `json:"sizeBytes"`
		} `json:"storage"`
		Traces struct {
			TraceCount float64 `json:"traceCount"`
			SpanCount  float64 `json:"spanCount"`
		} `json:"traces"`
		Logs struct {
			LogCount float64 `json:"logCount"`
		} `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(raw, &stats))
	assert.Equal(t, float64(1), stats.Traces.TraceCount)
	assert.Equal(t, float64(1), stats.Traces.SpanCount)
	assert.Equal(t, float64(1), stats.Logs.LogCount)
}

func TestClearLogs(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("clearLogs", nil))
	assert.NoError(t, err)
	assert.Equal(t, "Logs cleared successfully", result)

	searchResult, err := handler.Handle(ctx, createRequest("searchLogs", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &entries))
	assert.Len(t, entries, 0)
}

func TestClearMetrics(t *testing.T) {
	handler, teardown := setupHandlerWithMetrics(t)
	defer teardown()
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("clearMetrics", nil))
	assert.NoError(t, err)
	assert.Equal(t, "Metrics cleared successfully", result)

	searchResult, err := handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0)
}

func TestGetLogNotFound(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	req := createRequest("getLog", []string{"00000000-0000-0000-0000-0000000000aa"})
	result, err := handler.Handle(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrLogsNotFound, err)
}

func TestDeleteSpansByTraceID(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("deleteSpansByTraceID", []string{testTraceIDHex}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	searchResult, err := handler.Handle(ctx, createRequest("searchTraces", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0, "trace should be gone after delete")
}

func TestDeleteSpanByID(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	// The API serves span IDs in 16-char hex wire form; delete must round-trip it.
	result, err := handler.Handle(ctx, createRequest("deleteSpanByID", []string{"0000000000000001"}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	searchResult, err := handler.Handle(ctx, createRequest("searchTraces", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0, "the trace's only span should be gone after delete")
}

func TestDeleteLogByID(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	searchResult, err := handler.Handle(ctx, createRequest("searchLogs", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 1)
	logID, ok := entries[0]["id"].(string)
	require.True(t, ok)

	result, err := handler.Handle(ctx, createRequest("deleteLogByID", []string{logID}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	getResult, err := handler.Handle(ctx, createRequest("getLog", []string{logID}))
	assert.Nil(t, getResult)
	assert.Equal(t, ErrLogsNotFound, err, "deleted log should be gone")
}

// assertAttributeDiscovery unmarshals an attribute-discovery result and checks
// that service.name is reported as a string resource attribute.
func assertAttributeDiscovery(t *testing.T, result any) {
	t.Helper()
	raw, ok := result.(json.RawMessage)
	require.True(t, ok, "Expected json.RawMessage, got %T", result)
	var attrs []struct {
		Name           string `json:"name"`
		AttributeScope string `json:"attributeScope"`
		Type           string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(raw, &attrs))
	for _, a := range attrs {
		if a.Name == "service.name" && a.AttributeScope == "resource" {
			assert.Equal(t, "string", a.Type)
			return
		}
	}
	t.Errorf("service.name resource attribute not found in %s", string(raw))
}

func timeRangeParams() []string {
	now := time.Now().UnixNano()
	return []string{
		strconv.FormatInt(now-24*time.Hour.Nanoseconds(), 10),
		strconv.FormatInt(now+24*time.Hour.Nanoseconds(), 10),
	}
}

func TestGetLogAttributes(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributes", timeRangeParams()))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributes", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestGetMetricAttributes(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributes", timeRangeParams()))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributes", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestGetAttributesByTraceID(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getAttributesByTraceID", []string{testTraceIDHex}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("getAttributesByTraceID", []any{42}))
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidTraceID, err)
	})
}

func TestGetTraceSpanCount(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	t.Run("With Data", func(t *testing.T) {
		result, err := handler.Handle(ctx, createRequest("getTraceSpanCount", []string{testTraceIDHex}))
		assert.NoError(t, err)
		assert.Equal(t, int64(1), result)
	})

	t.Run("Unknown Trace", func(t *testing.T) {
		result, err := handler.Handle(ctx, createRequest("getTraceSpanCount", []string{"00000000-0000-0000-0000-0000000000aa"}))
		assert.NoError(t, err)
		assert.Equal(t, int64(0), result)
	})
}

// buildTestMetrics returns pmetric.Metrics with one gauge metric for handler tests.
func buildTestMetrics() pmetric.Metrics {
	base := time.Now().UnixNano()
	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-svc")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")
	met := sm.Metrics().AppendEmpty()
	met.SetName("test.gauge")
	met.SetDescription("A test gauge")
	met.SetUnit("bytes")
	g := met.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base))
	dp.SetDoubleValue(42.0)
	return m
}

func setupHandlerWithMetrics(t *testing.T) (*JSONRPCHandler, func()) {
	t.Helper()
	s, err := store.NewStore(context.Background(), "")
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s)
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTestMetrics())
	})
	require.NoError(t, err, "ingest metrics")

	return handler, func() { s.Close() }
}

func TestSearchMetricSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		req := createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Len(t, summaries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()

		req := createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1, "should return one metric summary")
		assert.Equal(t, "test.gauge", summaries[0]["name"])
		assert.Equal(t, "A test gauge", summaries[0]["description"])
		assert.Equal(t, "test-svc", summaries[0]["serviceName"])
		assert.Equal(t, "Gauge", summaries[0]["metricType"])
		assert.Equal(t, "bytes", summaries[0]["unit"])
		assert.NotEmpty(t, summaries[0]["id"])
		assert.NotNil(t, summaries[0]["seriesCount"])
		assert.NotNil(t, summaries[0]["lastValue"])
	})

	t.Run("With Query", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()

		query := map[string]any{
			"id":   "q1",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test.gauge",
			},
		}
		req := createRequest("searchMetricSummaries", []any{
			"0", strconv.FormatInt(1<<63-1, 10), query,
		})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)
		assert.Equal(t, "test.gauge", summaries[0]["name"])
	})
}

func TestGetMetric(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()

		summaryReq := createRequest("searchMetricSummaries", []string{
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		summaryResult, err := handler.Handle(context.Background(), summaryReq)
		require.NoError(t, err)
		summaryRaw, ok := summaryResult.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(summaryRaw, &summaries))
		require.Len(t, summaries, 1)
		streamID, ok := summaries[0]["id"].(string)
		require.True(t, ok)
		require.NotEmpty(t, streamID)

		req := createRequest("getMetric", []any{
			streamID, "0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		assert.Equal(t, "test.gauge", metric["name"])
		assert.Equal(t, "bytes", metric["unit"])
		// MetricData is now grouped by timeseries (per attribute set)
		// rather than a flat datapoint list. Each timeseries owns the
		// attributes for its group plus the pure-OTLP datapoints.
		timeseries, _ := metric["timeseries"].([]any)
		require.Len(t, timeseries, 1, "should have one timeseries")
		ts, _ := timeseries[0].(map[string]any)
		require.NotNil(t, ts, "timeseries must be a JSON object")
		assert.Contains(t, ts, "attributesKey", "timeseries should expose its grouping key")
		assert.Contains(t, ts, "attributes", "timeseries should own its attribute set")
		dps, _ := ts["datapoints"].([]any)
		assert.Len(t, dps, 1, "should have one datapoint inside the timeseries")
	})

	t.Run("Not Found", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()

		req := createRequest("getMetric", []any{
			"00000000-0000-0000-0000-000000000000",
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrMetricNotFound, err,
			"not-found must use the shared error convention, not a null result")
	})
}
