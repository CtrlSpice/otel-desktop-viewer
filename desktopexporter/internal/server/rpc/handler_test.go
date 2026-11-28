package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

func setupHandler(t *testing.T) *Handler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewHandler(s, zap.NewNop())
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
	rec.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	rec.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	rec.Body().SetStr("test log message")
	rec.SetEventName("request.failed")
	rec.SetSeverityText("INFO")
	rec.SetSeverityNumber(plog.SeverityNumberInfo)
	return logs
}

func setupHandlerWithData(t *testing.T) *Handler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	handler := NewHandler(s, zap.NewNop())
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildTestTraces())
	})
	assert.NoError(t, err, "ingest spans")

	err = s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, buildTestLogs())
	})
	assert.NoError(t, err, "ingest logs")

	return handler
}

func createRequest(method string, params any) *jsonrpc2.Request {
	paramsBytes, _ := json.Marshal(params)
	return &jsonrpc2.Request{
		Method: method,
		Params: paramsBytes,
		ID:     jsonrpc2.Int64ID(1),
	}
}

type rpcNullableRangeCase struct {
	name       string
	start, end any
	named      bool
}

func rpcNullableRangeCases() []rpcNullableRangeCase {
	maxTime := strconv.FormatInt(1<<63-1, 10)
	return []rpcNullableRangeCase{
		{"positional bounded", "0", maxTime, false},
		{"positional null/null", nil, nil, false},
		{"positional start only", "0", nil, false},
		{"positional end only", nil, maxTime, false},
		{"named bounded", "0", maxTime, true},
		{"named null/null", nil, nil, true},
		{"named start only", "0", nil, true},
		{"named end only", nil, maxTime, true},
	}
}

func searchRangeParams(tc rpcNullableRangeCase) any {
	if !tc.named {
		return []any{tc.start, tc.end}
	}
	// limit deliberately leaves query absent, forcing normalizeParams to retain
	// the interior null slot before the later named parameter.
	return map[string]any{"startTime": tc.start, "endTime": tc.end, "limit": 1}
}

func metricRangeParams(metricRef string, tc rpcNullableRangeCase) any {
	if !tc.named {
		return []any{metricRef, tc.start, tc.end}
	}
	// viewBuckets exercises all optional null holes between the range and a
	// later named parameter.
	return map[string]any{
		"metricRef": metricRef, "startTime": tc.start, "endTime": tc.end,
		"viewBuckets": 0,
	}
}

const testTraceIDHex = "00000000000000000000000000000001"

func TestSearchHandlersAcceptNullableBoundsPositionallyAndByName(t *testing.T) {
	handler := setupHandlerWithData(t)

	for _, method := range []struct {
		name   string
		assert func(*testing.T, json.RawMessage)
	}{
		{"searchTraceSummaries", func(t *testing.T, raw json.RawMessage) {
			var got []map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Len(t, got, 1)
			require.Equal(t, testTraceIDHex, got[0]["traceID"])
		}},
		{"searchLogSummaries", func(t *testing.T, raw json.RawMessage) {
			var got []map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Len(t, got, 1)
			require.Equal(t, "test log message", got[0]["bodyPreview"])
		}},
	} {
		t.Run(method.name, func(t *testing.T) {
			for _, tc := range rpcNullableRangeCases() {
				t.Run(tc.name, func(t *testing.T) {
					result, err := handler.Handle(context.Background(), createRequest(method.name, searchRangeParams(tc)))
					require.NoError(t, err)
					method.assert(t, result.(json.RawMessage))
				})
			}
		})
	}

	metricHandler := setupHandlerWithMetrics(t)
	for _, tc := range rpcNullableRangeCases() {
		t.Run("searchMetricSummaries/"+tc.name, func(t *testing.T) {
			result, err := metricHandler.Handle(context.Background(), createRequest("searchMetricSummaries", searchRangeParams(tc)))
			require.NoError(t, err)
			var got []map[string]any
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &got))
			require.Len(t, got, 1)
			require.Equal(t, "test.gauge", got[0]["name"])
			require.Equal(t, float64(1), got[0]["dataPointCount"])
		})
	}
}

func TestDetailSnapshotExcludesConcurrentIngest(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	result, err := storeSnapshotRead(ctx, handler.store, func(tx *sql.Tx) (struct {
		logCount, spanMatchCount int64
	}, error) {
		if _, err := spans.GetTraceOverview(ctx, tx, testTraceIDHex); err != nil {
			return struct{ logCount, spanMatchCount int64 }{}, err
		}
		if _, matchCount, err := spans.GetSpanSummaries(ctx, tx, 1, 2); err != nil || matchCount != 1 {
			return struct{ logCount, spanMatchCount int64 }{}, errors.Join(err, fmt.Errorf("initial span match count: %d", matchCount))
		}

		if err := handler.store.WithConn(func(conn driver.Conn) error {
			if err := logs.Ingest(ctx, conn, buildTestLogs()); err != nil {
				return err
			}
			duplicate := buildTestTraces()
			duplicate.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SetTraceID(
				pcommon.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
			)
			return spans.Ingest(ctx, conn, duplicate)
		}); err != nil {
			return struct{ logCount, spanMatchCount int64 }{}, err
		}

		logsRaw, err := logs.GetTraceLogSummaries(ctx, tx, testTraceIDHex)
		if err != nil {
			return struct{ logCount, spanMatchCount int64 }{}, err
		}
		var traceLogs []json.RawMessage
		if err := json.Unmarshal(logsRaw, &traceLogs); err != nil {
			return struct{ logCount, spanMatchCount int64 }{}, err
		}
		_, matchCount, err := spans.GetSpanSummaries(ctx, tx, 1, 2)
		return struct{ logCount, spanMatchCount int64 }{int64(len(traceLogs)), matchCount}, err
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.logCount)
	require.EqualValues(t, 1, result.spanMatchCount)

	require.NoError(t, handler.store.WithDBRead(func(db *sql.DB) error {
		logsRaw, err := logs.GetTraceLogSummaries(ctx, db, testTraceIDHex)
		if err != nil {
			return err
		}
		var traceLogs []json.RawMessage
		if err := json.Unmarshal(logsRaw, &traceLogs); err != nil {
			return err
		}
		_, matchCount, err := spans.GetSpanSummaries(ctx, db, 1, 2)
		require.Len(t, traceLogs, 2)
		require.EqualValues(t, 2, matchCount)
		return err
	}))
}

func serverMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func TestSearchSortParams(t *testing.T) {
	maxTime := strconv.FormatInt(1<<63-1, 10)
	valid := []struct {
		method string
		field  string
	}{
		{method: "searchTraceSummaries", field: "duration"},
		{method: "searchLogSummaries", field: "severity"},
		{method: "searchMetricSummaries", field: "dataPointCount"},
	}
	for _, tc := range valid {
		t.Run(tc.method+" accepts sort", func(t *testing.T) {
			handler := setupHandler(t)

			result, err := handler.Handle(context.Background(), createRequest(tc.method, []any{
				"0", maxTime, nil, 1,
				map[string]any{"field": tc.field, "direction": "desc"},
			}))
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))
		})

		t.Run(tc.method+" rejects unsupported field", func(t *testing.T) {
			handler := setupHandler(t)

			result, err := handler.Handle(context.Background(), createRequest(tc.method, []any{
				"0", maxTime, nil, 1,
				map[string]any{"field": "unsupported", "direction": "desc"},
			}))
			assert.Nil(t, result)
			assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		})
	}

	t.Run("rejects invalid direction", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("searchTraceSummaries", []any{
			"0", maxTime, nil, 1,
			map[string]any{"field": "duration", "direction": "descending"},
		}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})

	t.Run("rejects malformed object", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("searchTraceSummaries", []any{
			"0", maxTime, nil, 1,
			map[string]any{"field": "duration"},
		}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
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
		{"deleteLogsByRefs", ErrInvalidLogRef},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			handler := setupHandler(t)
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

			// Both spellings must pass validation. Note `count` is the
			// number of IDs supplied, not rows deleted (the store is empty
			// here, and these two spellings normalize to the same UUID);
			// functional round-trips live in TestDeleteSpanByID and friends.
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

			// 16-char hex is a span ID wire form; neither remaining delete
			// method accepts one.
			t.Run("16-Hex Wire Form", func(t *testing.T) {
				result, err := handler.Handle(ctx, createRequest(tc.method, []string{"0000000000000001"}))
				{
					assert.Nil(t, result)
					assert.Equal(t, tc.invalidErr, err)
				}
			})
		})
	}
}

// TestReadPathIDValidation covers the single-ID validation on the methods that
// take one ID rather than an array: a malformed ID returns the signal-specific
// code instead of reaching SQL and surfacing as a cast error dressed up as
// ErrInternal. Mostly reads, plus the single-reference deleteMetric method.
func TestReadPathIDValidation(t *testing.T) {
	handler := setupHandler(t)
	ctx := context.Background()

	cases := []struct {
		method     string
		params     any
		invalidErr error
	}{
		{"getTraceView", []string{"not-a-trace-id"}, ErrInvalidTraceID},
		{"getLog", []string{"not-a-log-id"}, ErrInvalidLogRef},
		{"getMetric", []string{"not-a-metric-id"}, ErrInvalidMetricRef},
		{"getMetricSeries", []any{"00000000-0000-0000-0000-000000000001", "not-a-series-id", nil, nil}, ErrInvalidMetricRef},
		{"getMetricView", []string{"not-a-metric-ref", "0", "1"}, ErrInvalidMetricRef},
		{"getTraceAttributeDefinitionsByTraceID", []string{"not-a-trace-id"}, ErrInvalidTraceID},
		{"getTraceSpanCount", []string{"not-a-trace-id"}, ErrInvalidTraceID},
		{"deleteMetric", []string{"not-a-metric-ref"}, ErrInvalidMetricRef},
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
	handler := setupHandler(t)

	req := createRequest("nonexistentMethod", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrMethodNotFound, err)
}

func TestGetStats(t *testing.T) {
	handler := setupHandlerWithData(t)

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

	var decoded struct {
		Storage map[string]any `json:"storage"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.NotContains(t, decoded.Storage, "inMemory")
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

func setupHandlerWithMetrics(t *testing.T) *Handler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	handler := NewHandler(s, zap.NewNop())
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTestMetrics())
	})
	require.NoError(t, err, "ingest metrics")

	return handler
}

func TestAttributeMethodsAcceptNoParams(t *testing.T) {
	handler := setupHandler(t)

	for _, method := range []string{"getTraceAttributeDefinitions", "getLogAttributeDefinitions", "getMetricAttributeDefinitions"} {
		t.Run(method, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), &jsonrpc2.Request{
				Method: method,
				ID:     jsonrpc2.Int64ID(1),
			})
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))

			result, err = handler.Handle(context.Background(), createRequest(method, []any{}))
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))

			result, err = handler.Handle(context.Background(), createRequest(method, map[string]any{}))
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))

			result, err = handler.Handle(context.Background(), createRequest(method, []any{nil, nil}))
			require.Nil(t, result)
			require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams,
				"attribute discovery accepts exactly zero parameters")

			result, err = handler.Handle(context.Background(), createRequest(method,
				map[string]any{"startTime": nil, "endTime": nil}))
			require.Nil(t, result)
			require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams,
				"attribute discovery has no named parameters")
		})
	}
}
