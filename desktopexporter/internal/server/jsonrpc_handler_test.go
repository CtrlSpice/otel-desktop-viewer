package server

import (
	"context"
	"encoding/json"
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

func setupHandler(t *testing.T) (*JSONRPCHandler, func()) {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s, zap.NewNop())
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
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s, zap.NewNop())
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildTestTraces(), s.FlushedIDs())
	})
	assert.NoError(t, err, "ingest spans")

	err = s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, buildTestLogs(), s.FlushedIDs())
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

type rpcNullableRangeCase struct {
	name       string
	start, end any
	named      bool
}

func rpcNullableRangeCases() []rpcNullableRangeCase {
	maxTime := strconv.FormatInt(1<<63-1, 10)
	return []rpcNullableRangeCase{
		{"positional null/null", nil, nil, false},
		{"positional start only", "0", nil, false},
		{"positional end only", nil, maxTime, false},
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

func metricRangeParams(streamID string, tc rpcNullableRangeCase) any {
	if !tc.named {
		return []any{streamID, tc.start, tc.end}
	}
	// viewBuckets exercises all optional null holes between the range and a
	// later named parameter.
	return map[string]any{
		"streamID": streamID, "startTime": tc.start, "endTime": tc.end,
		"viewBuckets": 0,
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

	t.Run("Garbage traceID in query tree", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		query := map[string]any{
			"id":   "q-garbage",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "not-a-trace",
			},
		}
		req := createRequest("searchTraces", []any{"0", strconv.FormatInt(1<<63-1, 10), query})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err, "garbage trace ID in search must not surface -32603")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Empty(t, summaries)
	})

	t.Run("Limit", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		maxTime := strconv.FormatInt(1<<63-1, 10)
		result, err := handler.Handle(context.Background(), createRequest("searchTraces", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchTraces", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestSearchHandlersAcceptNullableBoundsPositionallyAndByName(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()

	for _, method := range []struct {
		name   string
		assert func(*testing.T, json.RawMessage)
	}{
		{"searchTraces", func(t *testing.T, raw json.RawMessage) {
			var got []map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Len(t, got, 1)
			require.Equal(t, testTraceIDHex, got[0]["traceID"])
		}},
		{"searchLogs", func(t *testing.T, raw json.RawMessage) {
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

	metricHandler, metricTeardown := setupHandlerWithMetrics(t)
	defer metricTeardown()
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

	t.Run("Garbage spanID in query tree", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		query := map[string]any{
			"id":   "q-garbage",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "spanID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "zz-definitely-not-hex",
			},
		}
		req := createRequest("searchLogs", []any{"0", strconv.FormatInt(1<<63-1, 10), query})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err, "garbage span ID in search must not surface -32603")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Empty(t, entries)
	})

	t.Run("Limit", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()
		maxTime := strconv.FormatInt(1<<63-1, 10)

		result, err := handler.Handle(context.Background(), createRequest("searchLogs", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchLogs", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestSearchSortParams(t *testing.T) {
	maxTime := strconv.FormatInt(1<<63-1, 10)
	valid := []struct {
		method string
		field  string
	}{
		{method: "searchTraces", field: "duration"},
		{method: "searchLogs", field: "severity"},
		{method: "searchMetricSummaries", field: "dataPointCount"},
	}
	for _, tc := range valid {
		t.Run(tc.method+" accepts sort", func(t *testing.T) {
			handler, teardown := setupHandler(t)
			defer teardown()

			result, err := handler.Handle(context.Background(), createRequest(tc.method, []any{
				"0", maxTime, nil, 1,
				map[string]any{"field": tc.field, "direction": "desc"},
			}))
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))
		})

		t.Run(tc.method+" rejects unsupported field", func(t *testing.T) {
			handler, teardown := setupHandler(t)
			defer teardown()

			result, err := handler.Handle(context.Background(), createRequest(tc.method, []any{
				"0", maxTime, nil, 1,
				map[string]any{"field": "unsupported", "direction": "desc"},
			}))
			assert.Nil(t, result)
			assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		})
	}

	t.Run("rejects invalid direction", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("searchTraces", []any{
			"0", maxTime, nil, 1,
			map[string]any{"field": "duration", "direction": "descending"},
		}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})

	t.Run("rejects malformed object", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		result, err := handler.Handle(context.Background(), createRequest("searchTraces", []any{
			"0", maxTime, nil, 1,
			map[string]any{"field": "duration"},
		}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
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
		// ErrorIs, not Equal: the error now wraps the sentinel so it can say
		// which parameter was wrong and what arrived. The code on the wire is
		// unchanged; only the message gained detail, and a caller who cannot
		// read this file needs that detail more than we need identity.
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		assert.Contains(t, err.Error(), "startTime")
		assert.Contains(t, err.Error(), "pumpkin")
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
// ErrInternal. Mostly reads, plus deleteMetricStream, which is single-ID
// because metrics address a stream by one uuid (see the handler comment).
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
		{"deleteMetricStream", []string{"not-a-stream-id"}, ErrInvalidStreamID},
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

func TestDeleteMetricStream(t *testing.T) {
	handler, teardown := setupHandlerWithMetrics(t)
	defer teardown()
	ctx := context.Background()

	maxNano := strconv.FormatInt(1<<63-1, 10)
	searchResult, err := handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.NotEmpty(t, summaries, "fixture must provide at least one metric stream")
	streamID, ok := summaries[0]["id"].(string)
	require.True(t, ok)
	before := len(summaries)

	result, err := handler.Handle(ctx, createRequest("deleteMetricStream", []string{streamID}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric stream deleted successfully", result)

	// The stream is gone from search, and only that stream went with it.
	searchResult, err = handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok = searchResult.(json.RawMessage)
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, before-1, "exactly one stream should be gone")
	for _, s := range summaries {
		assert.NotEqual(t, streamID, s["id"], "deleted stream must not reappear")
	}
}

// TestDeleteMetricStreamNotFound covers deleting a stream that does not exist.
// The cascade is a series of unconditional DELETEs, so this is a no-op rather
// than an error -- the UI relies on that when a poll races a delete.
func TestDeleteMetricStreamNotFound(t *testing.T) {
	handler, teardown := setupHandlerWithMetrics(t)
	defer teardown()

	result, err := handler.Handle(context.Background(),
		createRequest("deleteMetricStream", []string{"00000000-0000-0000-0000-0000000000ff"}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric stream deleted successfully", result)
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
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	handler := NewJSONRPCHandler(s, zap.NewNop())
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTestMetrics(), s.FlushedIDs())
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
				"field":         map[string]any{"name": "name", "searchScope": "field"},
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

	t.Run("Limit", func(t *testing.T) {
		handler, teardown := setupHandlerWithMetrics(t)
		defer teardown()
		maxTime := strconv.FormatInt(1<<63-1, 10)

		result, err := handler.Handle(context.Background(), createRequest("searchMetricSummaries", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchMetricSummaries", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
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

	// A known stream queried over a window with no datapoints is NOT a
	// not-found: it returns valid MetricData with an empty timeseries list.
	// Only an unknown stream ID gets ErrMetricNotFound (see subtest above).
	t.Run("Known Stream, Empty Window", func(t *testing.T) {
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

		// Test data is timestamped time.Now(); the window [0, 1] ns is
		// guaranteed to miss it.
		req := createRequest("getMetric", []any{streamID, "0", "1"})
		result, err := handler.Handle(context.Background(), req)

		require.NoError(t, err,
			"an empty window on a known stream must not be treated as not-found")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "Expected json.RawMessage, got %T", result)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		assert.Equal(t, "test.gauge", metric["name"])
		assert.Equal(t, "bytes", metric["unit"])
		timeseries, ok := metric["timeseries"].([]any)
		require.True(t, ok, "timeseries must be a JSON array, got %T", metric["timeseries"])
		assert.Empty(t, timeseries)
	})
}

func TestMetricHandlersAcceptNullableBoundsPositionallyAndByName(t *testing.T) {
	handler, teardown := setupHandlerWithMetrics(t)
	defer teardown()

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []any{nil, nil}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	for _, tc := range rpcNullableRangeCases() {
		t.Run("getMetric/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetric", metricRangeParams(streamID, tc)))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &got))
			require.Equal(t, "test.gauge", got["name"])
			require.Equal(t, float64(1), got["datapointCount"])

			lastSeen := got["lastSeenNs"].(string)
			window := got["window"].(map[string]any)
			require.Equal(t, true, window["fittedToData"])
			switch {
			case tc.start == nil && tc.end == nil:
				require.Equal(t, lastSeen, window["startNs"])
				require.Equal(t, lastSeen, window["endNs"])
			case tc.start != nil:
				require.Equal(t, "0", window["startNs"])
				require.Equal(t, lastSeen, window["endNs"])
			default:
				require.Equal(t, lastSeen, window["startNs"])
				require.Equal(t, strconv.FormatInt(1<<63-1, 10), window["endNs"])
			}
		})

		t.Run("getMetricAggregate/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetricAggregate", metricRangeParams(streamID, tc)))
			require.NoError(t, err)
			require.JSONEq(t,
				`{"aggregate":null,"scalarAggregate":{"selected":[],"all":[]}}`,
				string(result.(json.RawMessage)))
		})
	}
}

// searchAttributes is the value-first counterpart to the getXAttributes
// discovery methods: given text seen in the UI, which keys hold it. It takes no
// time range and no signal, because the dictionary it reads is shared by all
// three.
func TestSearchAttributes(t *testing.T) {
	call := func(t *testing.T, handler *JSONRPCHandler, params any) []map[string]any {
		t.Helper()
		result, err := handler.Handle(context.Background(), createRequest("searchAttributes", params))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "expected json.RawMessage, got %T", result)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("finds the key holding a value", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		got := call(t, handler, []string{"pumpkin"})
		require.NotEmpty(t, got, "a value present in the fixture must be found")

		var found map[string]any
		for _, m := range got {
			if m["name"] == "service.name" {
				found = m
			}
		}
		require.NotNil(t, found, "service.name should be among the matches")
		assert.Equal(t, "resource", found["attributeScope"])
		assert.Contains(t, found["sampleValues"], "pumpkin.pie")
	})

	t.Run("no match and empty term return an empty list", func(t *testing.T) {
		handler, teardown := setupHandlerWithData(t)
		defer teardown()

		assert.Empty(t, call(t, handler, []string{"no-such-text-anywhere"}))
		assert.Empty(t, call(t, handler, []string{""}),
			"an empty term is not a request for the whole dictionary")
	})

	t.Run("rejects malformed params", func(t *testing.T) {
		handler, teardown := setupHandler(t)
		defer teardown()

		for _, params := range []any{[]string{}, []string{"a", "b"}, []int{1}} {
			_, err := handler.Handle(context.Background(), createRequest("searchAttributes", params))
			assert.Error(t, err, "params %v", params)
		}
	})
}

// TestGetMetricAcceptsEveryParameter walks the parameter list one at a time.
//
// Every parameter past the third is optional and additive, which means the
// arity bound has to move each time one is added. It did not: the bound stayed
// at four while four more were added below it, so every request carrying them
// was rejected before reaching the code that reads them.
//
// Nothing caught that. The store tests call GetMetric directly and never touch
// this handler, and the handler tests only ever sent three parameters. It
// surfaced by opening the app, where selecting any metric failed with "invalid
// params" and an empty detail pane.
func TestGetMetricAcceptsEveryParameter(t *testing.T) {
	handler, teardown := setupHandlerWithMetrics(t)
	defer teardown()

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.NotEmpty(t, summaries)
	streamID := summaries[0]["id"].(string)
	maxTime := strconv.FormatInt(1<<63-1, 10)

	full := []any{
		streamID,         // 1 stream
		"0",              // 2 start
		maxTime,          // 3 end
		"100",            // 4 targetBuckets
		[]any{},          // 5 seriesIDs
		[]any{0.5, 0.95}, // 6 quantiles
		"0",              // 7 tzOffsetNs
		true,             // 8 fitToData
		"120",            // 9 viewBuckets
		"64",             // 10 sparklineBuckets
		[]any{},          // 11 selectedSeriesIDs
		[]any{},          // 12 datapointSeriesIDs
		"10",             // 13 datapointSeriesLimit
		"Europe/London",  // 14 tzName
	}

	for n := 3; n <= len(full); n++ {
		t.Run(fmt.Sprintf("%d params", n), func(t *testing.T) {
			for _, method := range []string{"getMetric", "getMetricAggregate"} {
				result, err := handler.Handle(context.Background(),
					createRequest(method, full[:n]))
				require.NoErrorf(t, err, "%s with %d parameters", method, n)
				require.NotNil(t, result)
			}
		})
	}

	// One past the end is still rejected, or the bound means nothing.
	_, err = handler.Handle(context.Background(),
		createRequest("getMetric", append(append([]any{}, full...), "extra")))
	assert.Error(t, err, "a parameter beyond the known list must be refused")
}

// TestTimestampParamsAcceptNumbersWithoutLosingPrecision covers the trap that
// numeric timestamps used to fall into.
//
// A nanosecond timestamp is around 1.8e18, far past float64's exact-integer
// limit of 2^53. Decoded the ordinary way, three of four realistic timestamps
// round -- by up to 65ns -- which would move a query boundary without failing.
// Params are decoded with UseNumber so a JSON number arrives as text and
// parses exactly; this proves the decode path, not just the parser.
func TestTimestampParamsAcceptNumbersWithoutLosingPrecision(t *testing.T) {
	h := &JSONRPCHandler{}

	// Values chosen because they do NOT survive a float64 round trip.
	lossy := []int64{
		1787348704416123456,
		1787348704416123457,
		1787277368394484963,
	}

	for _, want := range lossy {
		require.NotEqual(t, want, int64(float64(want)),
			"fixture must actually be lossy through float64, or it proves nothing")

		t.Run(fmt.Sprintf("number_%d", want), func(t *testing.T) {
			var params []any
			require.NoError(t, decodeParams(
				json.RawMessage(fmt.Sprintf(`[%d, %d]`, want, want)), &params))

			got, err := h.parseTimestampParam(params[0], "startTime")
			require.NoError(t, err)
			require.Equal(t, want, got, "decoded through a JSON number, exactly")
		})

		t.Run(fmt.Sprintf("string_%d", want), func(t *testing.T) {
			var params []any
			require.NoError(t, decodeParams(
				json.RawMessage(fmt.Sprintf(`["%d"]`, want)), &params))

			got, err := h.parseTimestampParam(params[0], "startTime")
			require.NoError(t, err)
			require.Equal(t, want, got, "the original string form still works")
		})
	}
}

// TestTimestampParamErrorsSayWhatIsWrong guards the half that matters to a
// caller who cannot read this file: the message, not just the code.
func TestTimestampParamErrorsSayWhatIsWrong(t *testing.T) {
	h := &JSONRPCHandler{}

	t.Run("float64 is refused rather than rounded", func(t *testing.T) {
		// Reaching the parser with a float64 means the decoder was bypassed and
		// the precision is already gone. Rounding it would hide that.
		_, err := h.parseTimestampParam(float64(1787348704416123456), "startTime")
		require.Error(t, err)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		require.Contains(t, err.Error(), "startTime")
		require.Contains(t, err.Error(), "float64")
	})

	t.Run("wrong type names the parameter and the type", func(t *testing.T) {
		_, err := h.parseTimestampParam(true, "endTime")
		require.Error(t, err)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		require.Contains(t, err.Error(), "endTime")
		require.Contains(t, err.Error(), "bool")
	})

	t.Run("unparseable text is quoted back", func(t *testing.T) {
		var params []any
		require.NoError(t, decodeParams(json.RawMessage(`["not-a-number"]`), &params))
		_, err := h.parseTimestampParam(params[0], "startTime")
		require.Error(t, err)
		require.Contains(t, err.Error(), `"not-a-number"`)
	})
}

func TestOptionalTimestampParam(t *testing.T) {
	h := &JSONRPCHandler{}

	got, err := h.parseOptionalTimestampParam(nil, "startTime")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = h.parseOptionalTimestampParam(json.Number("1787348704416123457"), "endTime")
	require.NoError(t, err)
	require.Equal(t, int64(1787348704416123457), *got)

	got, err = h.parseOptionalTimestampParam("42", "startTime")
	require.NoError(t, err)
	require.Equal(t, int64(42), *got)

	_, err = h.parseOptionalTimestampParam(true, "endTime")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)

	_, err = h.parseTimestampParam(nil, "targetBuckets")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams,
		"non-bound numeric fields must remain non-null")

	start, end := json.Number("10"), json.Number("20")
	for _, tc := range []struct {
		name             string
		startParam       any
		endParam         any
		wantStartPresent bool
		wantEndPresent   bool
	}{
		{"unbounded", nil, nil, false, false},
		{"end only", nil, end, false, true},
		{"start only", start, nil, true, false},
		{"bounded", start, end, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timeRange, err := h.parseTimeRange(tc.startParam, tc.endParam)
			require.NoError(t, err)
			require.Equal(t, tc.wantStartPresent, timeRange.Start != nil)
			require.Equal(t, tc.wantEndPresent, timeRange.End != nil)
		})
	}
}

func TestAttributeMethodsAcceptNoParams(t *testing.T) {
	handler, teardown := setupHandler(t)
	defer teardown()

	for _, method := range []string{"getTraceAttributes", "getLogAttributes", "getMetricAttributes"} {
		t.Run(method, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(method, []any{}))
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))

			result, err = handler.Handle(context.Background(), createRequest(method, []any{nil, nil}))
			require.NoError(t, err, "legacy two-parameter form accepts nullable bounds")
			require.JSONEq(t, `[]`, string(result.(json.RawMessage)))
		})
	}
}

// getFieldValues is the search box's value-completion source. Its guards are
// what keep an allowlist an allowlist, so they are pinned rather than left to
// the named-params table walk, which only checks the method has a name list.
func TestGetFieldValues(t *testing.T) {
	handler, teardown := setupHandlerWithData(t)
	defer teardown()
	ctx := context.Background()

	decode := func(t *testing.T, result any) []string {
		t.Helper()
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "expected json.RawMessage, got %T", result)
		var out []string
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("serves an allowlisted field", func(t *testing.T) {
		result, err := handler.Handle(ctx,
			createRequest("getFieldValues", []any{"traces", "name", "", 10}))
		require.NoError(t, err)
		assert.NotEmpty(t, decode(t, result))
	})

	t.Run("named params reach the same place", func(t *testing.T) {
		result, err := handler.Handle(ctx, createRequest("getFieldValues",
			map[string]any{
				"signal": "traces", "field": "name", "term": "", "limit": 10,
			}))
		require.NoError(t, err)
		assert.NotEmpty(t, decode(t, result))
	})

	t.Run("clamps a limit above the ceiling", func(t *testing.T) {
		// Not an error: a caller asking for everything gets a dropdown's
		// worth. The clamp is the only thing standing between a completion
		// keystroke and a full column dump.
		result, err := handler.Handle(ctx,
			createRequest("getFieldValues", []any{"traces", "name", "", 100000}))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(decode(t, result)), 500)
	})

	t.Run("clamps a limit below one", func(t *testing.T) {
		for _, limit := range []any{0, -5} {
			result, err := handler.Handle(ctx,
				createRequest("getFieldValues", []any{"traces", "name", "", limit}))
			require.NoError(t, err, "limit %v", limit)
			assert.Len(t, decode(t, result), 1, "limit %v", limit)
		}
	})

	t.Run("refuses a field outside the allowlist", func(t *testing.T) {
		// statusMessage is a real span column, deliberately not completable:
		// the allowlist is a boundary, not a convenience.
		_, err := handler.Handle(ctx,
			createRequest("getFieldValues", []any{"traces", "statusMessage", "", 10}))
		require.Error(t, err)
	})

	t.Run("refuses an unknown signal", func(t *testing.T) {
		_, err := handler.Handle(ctx,
			createRequest("getFieldValues", []any{"spans", "name", "", 10}))
		require.Error(t, err)
	})

	t.Run("refuses a malformed call", func(t *testing.T) {
		cases := []struct {
			name   string
			params any
		}{
			{"too few params", []any{"traces", "name", ""}},
			{"too many params", []any{"traces", "name", "", 10, "extra"}},
			{"non-string signal", []any{1, "name", "", 10}},
			{"non-string field", []any{"traces", 1, "", 10}},
			{"non-string term", []any{"traces", "name", 1, 10}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := handler.Handle(ctx, createRequest("getFieldValues", tc.params))
				assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
			})
		}
	})
}
