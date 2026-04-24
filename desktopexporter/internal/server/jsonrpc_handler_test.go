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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
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
		assert.Equal(t, "test log message", entries[0]["body"])
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

// TestSearchMetricsInvalidParams ensures searchMetrics with wrong param count returns ErrInvalidParams.
func TestSearchMetricsInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("searchMetrics", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}

// TestGetDatapointQuantilesInvalidParams covers the parsing/validation paths in
// getDatapointQuantiles. Happy-path flow + unsupported-type / found-but-wrong
// type errors are exercised in the metrics package's unit tests; here we just
// verify the handler's input contract.
func TestGetDatapointQuantilesInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	cases := []struct {
		name   string
		params any
	}{
		{"WrongCount", []any{"id-only"}},
		{"NonStringDatapointID", []any{42, []any{0.5}}},
		{"NonArrayQuantiles", []any{"abc", "not-an-array"}},
		{"NonNumberQuantile", []any{"abc", []any{"0.5"}}},
		{"NegativeQuantile", []any{"abc", []any{-0.1}}},
		{"AboveOneQuantile", []any{"abc", []any{1.1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := createRequest("getDatapointQuantiles", c.params)
			result, err := handler.Handle(context.Background(), req)
			assert.Nil(t, result)
			assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
		})
	}
}

// TestGetDatapointQuantilesNotFound verifies that an unknown datapoint UUID
// surfaces ErrDatapointNotFound rather than ErrInternal.
func TestGetDatapointQuantilesNotFound(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("getDatapointQuantiles", []any{
		"00000000-0000-0000-0000-000000000000",
		[]any{0.5},
	})
	result, err := handler.Handle(context.Background(), req)
	assert.Nil(t, result)
	assert.Equal(t, ErrDatapointNotFound, err)
}

// TestGetMetricQuantileSeriesInvalidParams covers the handler's input
// contract: the wrong shape of params should always come back as
// jsonrpc2.ErrInvalidParams without ever touching the store. Behavior of
// the underlying SQL pipeline is exercised in the metrics package tests.
func TestGetMetricQuantileSeriesInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	const startTs = "1700000000000000000"
	const endTs = "1700000060000000000"
	cases := []struct {
		name   string
		params any
	}{
		{"WrongCount", []any{"id-only", []any{0.5}, "per-stream", startTs, endTs}}, // 5
		{"NonStringMetricID", []any{42, []any{0.5}, "per-stream", startTs, endTs, 100.0}},
		{"NonArrayQuantiles", []any{"abc", "not-an-array", "per-stream", startTs, endTs, 100.0}},
		{"NonNumberQuantile", []any{"abc", []any{"0.5"}, "per-stream", startTs, endTs, 100.0}},
		{"NegativeQuantile", []any{"abc", []any{-0.1}, "per-stream", startTs, endTs, 100.0}},
		{"AboveOneQuantile", []any{"abc", []any{1.1}, "per-stream", startTs, endTs, 100.0}},
		{"NonStringMode", []any{"abc", []any{0.5}, 7, startTs, endTs, 100.0}},
		{"NonStringStartTs", []any{"abc", []any{0.5}, "per-stream", 1700000000000000000, endTs, 100.0}},
		{"NonStringEndTs", []any{"abc", []any{0.5}, "per-stream", startTs, 1700000060000000000, 100.0}},
		{"UnparseableStartTs", []any{"abc", []any{0.5}, "per-stream", "not-a-number", endTs, 100.0}},
		{"NonNumberMaxPoints", []any{"abc", []any{0.5}, "per-stream", startTs, endTs, "100"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := createRequest("getMetricQuantileSeries", c.params)
			result, err := handler.Handle(context.Background(), req)
			assert.Nil(t, result)
			assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
		})
	}
}

// TestGetMetricQuantileSeriesValidationDispatch verifies that the helper's
// own validation errors (invalid time range, invalid maxPoints, bad mode)
// surface as the right JSON-RPC error codes -- so the frontend can
// distinguish "your request is malformed" from "your data is in a state we
// can't aggregate".
func TestGetMetricQuantileSeriesValidationDispatch(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	t.Run("InvalidTimeRangeMapsTo32011", func(t *testing.T) {
		// metricID is irrelevant when start/end fail validation up front.
		req := createRequest("getMetricQuantileSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			[]any{0.5},
			"per-stream",
			"1700000060000000000", // start
			"1700000000000000000", // end < start
			100.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidTimeRange, err)
	})

	t.Run("InvalidMaxPointsMapsTo32012", func(t *testing.T) {
		req := createRequest("getMetricQuantileSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			[]any{0.5},
			"per-stream",
			"1700000000000000000",
			"1700000060000000000",
			0.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidMaxPoints, err)
	})

	t.Run("InvalidModeMapsToInvalidParams", func(t *testing.T) {
		// Bad mode is a client-side mistake; we surface it as the standard
		// InvalidParams rather than a custom code.
		req := createRequest("getMetricQuantileSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			[]any{0.5},
			"not-a-real-mode",
			"1700000000000000000",
			"1700000060000000000",
			100.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		// ErrInvalidQuantileSeriesMode short-circuits before any store access,
		// so ErrMetricNotFound never enters the picture.
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

// TestGetMetricQuantileSeriesNotFound verifies that an unknown metric UUID
// surfaces ErrMetricNotFound rather than ErrInternal. Empty-quantiles
// short-circuits before any store access, so we send a non-empty list.
func TestGetMetricQuantileSeriesNotFound(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("getMetricQuantileSeries", []any{
		"00000000-0000-0000-0000-000000000000",
		[]any{0.5},
		"per-stream",
		"1700000000000000000",
		"1700000060000000000",
		100.0,
	})
	result, err := handler.Handle(context.Background(), req)
	assert.Nil(t, result)
	assert.Equal(t, ErrMetricNotFound, err)
}

// ---------------------------------------------------------------------------
// getMetricBucketSeries handler tests
// ---------------------------------------------------------------------------

func TestGetMetricBucketSeriesInvalidParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	const startTs = "1700000000000000000"
	const endTs = "1700000060000000000"
	cases := []struct {
		name   string
		params any
	}{
		{"WrongCount", []any{"id-only", "per-stream", startTs, endTs}},
		{"NonStringMetricID", []any{42, "per-stream", startTs, endTs, 100.0}},
		{"NonStringMode", []any{"abc", 7, startTs, endTs, 100.0}},
		{"NonStringStartTs", []any{"abc", "per-stream", 1700000000000000000, endTs, 100.0}},
		{"NonStringEndTs", []any{"abc", "per-stream", startTs, 1700000060000000000, 100.0}},
		{"UnparseableStartTs", []any{"abc", "per-stream", "not-a-number", endTs, 100.0}},
		{"NonNumberMaxPoints", []any{"abc", "per-stream", startTs, endTs, "100"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := createRequest("getMetricBucketSeries", c.params)
			result, err := handler.Handle(context.Background(), req)
			assert.Nil(t, result)
			assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
		})
	}
}

func TestGetMetricBucketSeriesValidationDispatch(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	t.Run("InvalidTimeRangeMapsTo32011", func(t *testing.T) {
		req := createRequest("getMetricBucketSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			"per-stream",
			"1700000060000000000",
			"1700000000000000000",
			100.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidTimeRange, err)
	})

	t.Run("InvalidMaxPointsMapsTo32012", func(t *testing.T) {
		req := createRequest("getMetricBucketSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			"per-stream",
			"1700000000000000000",
			"1700000060000000000",
			0.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidMaxPoints, err)
	})

	t.Run("InvalidModeMapsToInvalidParams", func(t *testing.T) {
		req := createRequest("getMetricBucketSeries", []any{
			"00000000-0000-0000-0000-000000000000",
			"not-a-real-mode",
			"1700000000000000000",
			"1700000060000000000",
			100.0,
		})
		result, err := handler.Handle(context.Background(), req)
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestGetMetricBucketSeriesNotFound(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	req := createRequest("getMetricBucketSeries", []any{
		"00000000-0000-0000-0000-000000000000",
		"per-stream",
		"1700000000000000000",
		"1700000060000000000",
		100.0,
	})
	result, err := handler.Handle(context.Background(), req)
	assert.Nil(t, result)
	assert.Equal(t, ErrMetricNotFound, err)
}
