package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
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

func setupHandler(t *testing.T) *JSONRPCHandler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewJSONRPCHandler(s, zap.NewNop())
}

func TestQueryUsesDefaultAndExplicitLimits(t *testing.T) {
	handler := setupHandler(t)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select * from range(26)"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var response struct {
		Rows      [][]json.RawMessage `json:"rows"`
		Truncated bool                `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	assert.Len(t, response.Rows, 25)
	assert.True(t, response.Truncated)

	result, err = handler.Handle(context.Background(), createRequest("query", map[string]any{"sql": "SHOW TABLES", "limit": 2}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"columns":[`)
	assert.Contains(t, string(encoded), `"truncated":`)

	result, err = handler.Handle(context.Background(), createRequest("query", []any{"DESCRIBE spans"}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"column_name"`)
}

func TestQueryPreservesNativeIntegerAndTopLevelJSON(t *testing.T) {
	handler := setupHandler(t)
	result, err := handler.Handle(context.Background(), createRequest("query", []any{
		`select 9007199254740993::bigint as n, json('{"n":9007199254740993}') as payload`,
	}))
	require.NoError(t, err)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[9007199254740993,{"n":9007199254740993}]]`)
}

func TestQueryReadsViewerTablesAndMacros(t *testing.T) {
	handler := setupHandlerWithData(t)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{`
		select l.event_name,
		       body_preview(l.body) as preview,
		       trace_id_wire(l.trace_id) as trace_id,
		       span_id_wire(l.span_id) as span_id,
		       attrs_json(r.attribute_ids) as resource_attributes
		from logs l
		join resources r on r.id = l.resource_id
		where l.service_name = 'pumpkin.pie'`,
	}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Rows [][]json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	require.Len(t, response.Rows, 1)
	require.Len(t, response.Rows[0], 5)
	assert.JSONEq(t, `"request.failed"`, string(response.Rows[0][0]))
	assert.JSONEq(t, `"test log message"`, string(response.Rows[0][1]))
	assert.JSONEq(t, `"00000000000000000000000000000001"`, string(response.Rows[0][2]))
	assert.JSONEq(t, `"0000000000000001"`, string(response.Rows[0][3]))
	assert.Contains(t, string(response.Rows[0][4]), `"key":"service.name"`)
	assert.Contains(t, string(response.Rows[0][4]), `"value":{"kind":"string","value":"pumpkin.pie"}`)

	result, err = handler.Handle(context.Background(), createRequest("query", map[string]any{
		"sql": `select count(*)::bigint as matched
			from spans s
			join logs l on l.trace_id = s.trace_id
			where s.service_name = 'pumpkin.pie'`,
	}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"columns":[{"name":"matched","type":"BIGINT"}],
		"rows":[[1]],
		"truncated":false
	}`, string(encoded))
}

func TestQueryPublicResponseCoversLimitBoundariesAndDuplicateColumns(t *testing.T) {
	handler := setupHandler(t)
	for _, tc := range []struct {
		name   string
		params any
		want   string
	}{
		{name: "zero", params: []any{"select 1", 0}, want: `{"columns":[{"name":"1","type":"INTEGER"}],"rows":[],"truncated":true}`},
		{name: "maximum", params: map[string]any{"sql": "select 1", "limit": uint64(math.MaxUint64)}, want: `{"columns":[{"name":"1","type":"INTEGER"}],"rows":[[1]],"truncated":false}`},
		{name: "empty duplicate columns", params: []any{"select 1 as duplicate, 2 as duplicate where false"}, want: `{"columns":[{"name":"duplicate","type":"INTEGER"},{"name":"duplicate","type":"INTEGER"}],"rows":[],"truncated":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest("query", tc.params))
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestQueryPublicCancellationLeavesStoreReusable(t *testing.T) {
	handler := setupHandler(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := handler.Handle(ctx, createRequest("query", []any{
		"select sum(i) from range(1000000000) input(i)",
	}))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestCanceled)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select 1"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[1]]`)
}

func TestQueryPreparationCancellationUsesRequestCanceled(t *testing.T) {
	handler := setupHandler(t)
	statement := "SELECT " + strings.Repeat("1,", 100_000) + "1"
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	result, err := handler.Handle(ctx, createRequest("query", []any{statement}))

	assert.Nil(t, result)
	assert.Equal(t, ErrRequestCanceled, err)
	assert.False(t, errors.Is(err, jsonrpc2.ErrInvalidParams), "%v", err)
}

func TestQueryExplicitCancellationLeavesStoreReusable(t *testing.T) {
	handler := setupHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()

	_, err := handler.Handle(ctx, createRequest("query", []any{
		"select sum(i) from range(1000000000) input(i)",
	}))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.ErrorIs(t, err, ErrRequestCanceled)
	assert.NotErrorIs(t, err, ErrInvalidQuery)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select 1"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[1]]`)
}

func TestQueryPublicReadsReopenedPersistentStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "viewer.db")
	initial, err := store.NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, initial.Close())

	reopened, err := store.NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	handler := NewJSONRPCHandler(reopened, zap.NewNop())
	result, err := handler.Handle(context.Background(), createRequest("query", map[string]any{
		"sql": "select count(*)::bigint as span_count from spans",
	}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[0]]`)
}

func TestQueryRejectsInvalidParametersAndWrites(t *testing.T) {
	handler := setupHandler(t)
	for _, params := range []any{
		[]any{},
		[]any{"select 1", 1, 2},
		[]any{1},
		[]any{"select 1", nil},
		[]any{"select 1", -1},
		[]any{"select 1", 1.5},
		[]any{"delete from spans"},
		[]any{"select 1; select 2"},
	} {
		_, err := handler.Handle(context.Background(), createRequest("query", params))
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams, "params %#v: %v", params, err)
	}

	_, err := handler.Handle(context.Background(), createRequest("query", []any{
		"select cast(value as integer) from (values ('failed')) input(value)",
	}))
	assert.ErrorIs(t, err, ErrInvalidQuery)
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

func setupHandlerWithData(t *testing.T) *JSONRPCHandler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
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

func TestSearchTraceSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("searchTraceSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Len(t, summaries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		req := createRequest("searchTraceSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1, "searchTraceSummaries should return the ingested trace")
		assert.Equal(t, testTraceIDHex, summaries[0]["traceID"])
	})

	t.Run("Garbage traceID in query tree", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		query := map[string]any{
			"id":   "q-garbage",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "not-a-trace",
			},
		}
		req := createRequest("searchTraceSummaries", []any{"0", strconv.FormatInt(1<<63-1, 10), query})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err, "garbage trace ID in search must not surface -32603")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Empty(t, summaries)
	})

	t.Run("Limit", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		maxTime := strconv.FormatInt(1<<63-1, 10)
		result, err := handler.Handle(context.Background(), createRequest("searchTraceSummaries", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchTraceSummaries", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

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

func TestGetTraceView(t *testing.T) {
	handler := setupHandlerWithData(t)

	t.Run("Found", func(t *testing.T) {
		req := createRequest("getTraceView", []string{testTraceIDHex})
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
		req := createRequest("getTraceView", []string{"00000000-0000-0000-0000-000000000099"})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrTraceNotFound, err)
	})
}

func TestClearTraces(t *testing.T) {
	handler := setupHandlerWithData(t)

	req := createRequest("clearTraces", nil)
	result, err := handler.Handle(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, "Traces cleared successfully", result)

	searchReq := createRequest("searchTraceSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
	searchResult, searchErr := handler.Handle(context.Background(), searchReq)
	assert.NoError(t, searchErr)
	raw, ok := searchResult.(json.RawMessage)
	assert.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0)
}

func TestSearchLogSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Len(t, entries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		req := createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1, "searchLogSummaries should return the ingested log")
		// The summary carries bodyPreview; getLog carries the complete body.
		assert.Equal(t, "test log message", entries[0]["bodyPreview"])

		logRef, ok := entries[0]["logRef"].(string)
		require.True(t, ok, "summary should carry an id for detail fetch")
		getReq := createRequest("getLog", []string{logRef})
		getResult, getErr := handler.Handle(context.Background(), getReq)
		assert.NoError(t, getErr)
		getRaw, ok := getResult.(json.RawMessage)
		assert.True(t, ok)
		var full map[string]any
		assert.NoError(t, json.Unmarshal(getRaw, &full))
		assert.Equal(t, map[string]any{"kind": "string", "value": "test log message"}, full["body"])
	})

	t.Run("Garbage spanID in query tree", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		query := map[string]any{
			"id":   "q-garbage",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "spanID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "zz-definitely-not-hex",
			},
		}
		req := createRequest("searchLogSummaries", []any{"0", strconv.FormatInt(1<<63-1, 10), query})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err, "garbage span ID in search must not surface -32603")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Empty(t, entries)
	})

	t.Run("Limit", func(t *testing.T) {
		handler := setupHandlerWithData(t)
		maxTime := strconv.FormatInt(1<<63-1, 10)

		result, err := handler.Handle(context.Background(), createRequest("searchLogSummaries", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchLogSummaries", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestGetTraceLogSummaries(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", map[string]any{
			"traceID": testTraceIDHex,
		}))
		require.NoError(t, err)
		var entries []map[string]any
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &entries))
		require.Len(t, entries, 1)
		require.Equal(t, "0000000000000001", entries[0]["spanID"])
		require.Equal(t, "request.failed", entries[0]["eventName"])
		require.Equal(t, "test log message", entries[0]["bodyPreview"])
		require.NotContains(t, entries[0], "body")
		require.NotContains(t, entries[0], "attributes")
	})

	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{
			"00000000000000000000000000000002",
		}))
		require.NoError(t, err)
		require.JSONEq(t, `[]`, string(result.(json.RawMessage)))
	})

	t.Run("Malformed trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{"not-a-trace-id"}))
		require.Nil(t, result)
		require.Equal(t, ErrInvalidTraceID, err)
	})

	t.Run("Empty trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{""}))
		require.Nil(t, result)
		require.Equal(t, ErrInvalidTraceID, err)
	})

	t.Run("Missing trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{}))
		require.Nil(t, result)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestGetTraceReturnsOnlyCompactOverviewFields(t *testing.T) {
	handler := setupHandlerWithData(t)
	result, err := handler.Handle(context.Background(), createRequest("getTraceOverview", map[string]any{"traceID": testTraceIDHex}))
	require.NoError(t, err)
	overview, ok := result.(compactTraceResult)
	require.True(t, ok, "expected compactTraceResult, got %T", result)
	require.Equal(t, testTraceIDHex, overview.Trace.TraceID)
	require.EqualValues(t, len(overview.Spans), overview.Trace.SpanCount)
	require.Equal(t, len(overview.Logs), overview.Trace.LogCount)
	require.NotEmpty(t, overview.Trace.StartTime)
	require.NotEmpty(t, overview.Trace.DurationNs)
	require.NotEmpty(t, overview.Spans)
	require.Equal(t, "pumpkin.pie", overview.Spans[0].Service)
	require.Len(t, overview.Logs, 1)
	require.Equal(t, "INFO", overview.Logs[0].Severity)
	require.Equal(t, "test log message", overview.Logs[0].Body)

	encoded, err := json.Marshal(overview)
	require.NoError(t, err)
	var shape map[string]any
	require.NoError(t, json.Unmarshal(encoded, &shape))
	require.ElementsMatch(t, []string{"trace", "spans", "logs"}, serverMapKeys(shape))
	require.ElementsMatch(t, []string{"traceID", "spanCount", "logCount", "startTime", "durationNs"}, serverMapKeys(shape["trace"].(map[string]any)))
	require.ElementsMatch(t, []string{"spanID", "parentSpanID", "service", "name", "startOffsetNs", "durationNs"}, serverMapKeys(shape["spans"].([]any)[0].(map[string]any)))
	require.ElementsMatch(t, []string{"timestamp", "spanID", "severity", "service", "eventName", "body"}, serverMapKeys(shape["logs"].([]any)[0].(map[string]any)))
	require.NotContains(t, string(encoded), "attributes")
	require.NotContains(t, string(encoded), "severityNumber")
	require.NotContains(t, string(encoded), "schemaURL")
}

func TestGetTraceComputesExactUnsignedTimingAndStableOrder(t *testing.T) {
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "exact")
	ss := rs.ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}
	maximum := ^uint64(0)
	later := ss.Spans().AppendEmpty()
	later.SetTraceID(traceID)
	later.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 2})
	later.SetName("later")
	later.SetStartTimestamp(pcommon.Timestamp(maximum - 500))
	later.SetEndTimestamp(pcommon.Timestamp(maximum - 505))
	earlier := ss.Spans().AppendEmpty()
	earlier.SetTraceID(traceID)
	earlier.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	earlier.SetName("earlier")
	earlier.SetStartTimestamp(pcommon.Timestamp(maximum - 1000))
	earlier.SetEndTimestamp(pcommon.Timestamp(maximum - 900))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, traces, s.FlushedIDs())
	}))

	result, err := NewJSONRPCHandler(s, zap.NewNop()).Handle(context.Background(), createRequest("getTraceOverview", []string{"00000000000000000000000000000003"}))
	require.NoError(t, err)
	overview := result.(compactTraceResult)
	require.Equal(t, "18446744073709550615", overview.Trace.StartTime)
	require.Equal(t, "495", overview.Trace.DurationNs)
	require.Len(t, overview.Spans, 2)
	require.Equal(t, "0000000000000001", overview.Spans[0].SpanID)
	require.Equal(t, "0", overview.Spans[0].StartOffsetNs)
	require.Equal(t, "100", overview.Spans[0].DurationNs)
	require.Equal(t, "0000000000000002", overview.Spans[1].SpanID)
	require.Equal(t, "500", overview.Spans[1].StartOffsetNs)
	require.Equal(t, "-5", overview.Spans[1].DurationNs)
}

func TestGetTraceValidatesIDAndContext(t *testing.T) {
	handler := setupHandler(t)
	result, err := handler.Handle(context.Background(), createRequest("getTraceOverview", []string{"not-a-trace-id"}))
	require.Nil(t, result)
	require.Equal(t, ErrInvalidTraceID, err)

	result, err = handler.Handle(context.Background(), createRequest("getTraceOverview", []string{"00000000000000000000000000000002"}))
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrTraceNotFound)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = setupHandlerWithData(t).Handle(ctx, createRequest("getTraceOverview", []string{testTraceIDHex}))
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRequestCanceled)
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
			if err := logs.Ingest(ctx, conn, buildTestLogs(), handler.store.FlushedIDs()); err != nil {
				return err
			}
			duplicate := buildTestTraces()
			duplicate.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SetTraceID(
				pcommon.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
			)
			return spans.Ingest(ctx, conn, duplicate, handler.store.FlushedIDs())
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

func TestCompactTraceLogsPreservesNullAndDanglingSpanIDs(t *testing.T) {
	logs, err := compactTraceLogs(json.RawMessage(`[
		{"timestamp":"18446744073709551615","spanID":null,"severityText":"","severityNumber":9,"serviceName":"api","eventName":"","bodyPreview":"preview"},
		{"timestamp":"18446744073709551614","spanID":"ffffffffffffffff","severityText":"CUSTOM","severityNumber":0,"serviceName":"worker","eventName":"retry","bodyPreview":"dangling"}
	]`))
	require.NoError(t, err)
	require.Len(t, logs, 2)
	require.Nil(t, logs[0].SpanID)
	require.Equal(t, "INFO", logs[0].Severity)
	require.Equal(t, "preview", logs[0].Body)
	require.NotNil(t, logs[1].SpanID)
	require.Equal(t, "ffffffffffffffff", *logs[1].SpanID)
	require.Equal(t, "CUSTOM", logs[1].Severity)
}

func TestLogSeverityLabelUsesReceivedTextOrExactNumberBand(t *testing.T) {
	for _, tc := range []struct {
		text, number, want string
	}{
		{"CUSTOM", "not-a-number", "CUSTOM"},
		{"", "0", "UNSPECIFIED"},
		{"", "1", "TRACE"},
		{"", "4", "TRACE"},
		{"", "5", "DEBUG"},
		{"", "24", "FATAL"},
		{"", "25", "Unknown (25)"},
		{"", "-1", "Unknown (-1)"},
	} {
		got, err := logSeverityLabel(tc.text, json.Number(tc.number))
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	_, err := logSeverityLabel("", json.Number("not-a-number"))
	require.ErrorContains(t, err, "decode log severity number")
}

func TestGetSpanResolutionNeverGuesses(t *testing.T) {
	handler := setupHandler(t)
	spanID := pcommon.SpanID{7: 42}
	data := ptrace.NewTraces()
	for _, last := range []byte{4, 2, 3, 1} {
		rs := data.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: last})
		span.SetSpanID(spanID)
		span.SetName(fmt.Sprintf("trace-%d", last))
		span.SetStartTimestamp(1)
		span.SetEndTimestamp(2)
	}
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, data, handler.store.FlushedIDs())
	}))

	result, err := handler.Handle(context.Background(), createRequest("getSpan", map[string]any{"spanID": "000000000000002A"}))
	require.NoError(t, err)
	ambiguous := result.(spanAmbiguousResult)
	require.EqualValues(t, 4, ambiguous.MatchCount)
	var summaries []struct {
		TraceID string `json:"traceID"`
		SpanID  string `json:"spanID"`
		Name    string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(ambiguous.Summaries, &summaries))
	require.Equal(t, []string{
		"00000000000000000000000000000001",
		"00000000000000000000000000000002",
		"00000000000000000000000000000003",
		"00000000000000000000000000000004",
	}, []string{summaries[0].TraceID, summaries[1].TraceID, summaries[2].TraceID, summaries[3].TraceID})
	require.Equal(t, "000000000000002a", summaries[0].SpanID)
	require.Equal(t, "trace-1", summaries[0].Name)
	require.False(t, ambiguous.Truncated)
	require.Equal(t, "000000000000002a", ambiguous.SpanID)

	result, err = handler.Handle(context.Background(), createRequest("getSpan", []any{"000000000000002a", "00000000000000000000000000000003"}))
	require.NoError(t, err)
	found := result.(spanFoundResult)
	require.Equal(t, "00000000000000000000000000000003", found.TraceID)
	require.Contains(t, string(found.Span), `"name":"trace-3"`)

	require.NoError(t, handler.store.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`delete from spans where trace_id in (?::uuid, ?::uuid)`,
			"00000000000000000000000000000003", "00000000000000000000000000000004")
		return err
	}))
	result, err = handler.Handle(context.Background(), createRequest("getSpan", []any{"000000000000002a"}))
	require.NoError(t, err)
	require.EqualValues(t, 2, result.(spanAmbiguousResult).MatchCount)

	require.NoError(t, handler.store.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`delete from spans where trace_id = ?::uuid`, "00000000000000000000000000000002")
		return err
	}))
	result, err = handler.Handle(context.Background(), createRequest("getSpan", []any{"000000000000002a"}))
	require.NoError(t, err)
	require.Equal(t, "00000000000000000000000000000001", result.(spanFoundResult).TraceID)

	result, err = handler.Handle(context.Background(), createRequest("getSpan", []any{"000000000000002a", "00000000000000000000000000000009"}))
	require.NoError(t, err)
	missing := result.(spanNotFoundResult)
	require.Equal(t, "notFound", missing.Status)
	require.NotNil(t, missing.TraceID)
	require.Equal(t, "00000000000000000000000000000009", *missing.TraceID)
}

func TestGetSpanAmbiguityUsesLookaheadAndExactCount(t *testing.T) {
	handler := setupHandler(t)
	data := ptrace.NewTraces()
	for i := byte(1); i <= 30; i++ {
		rs := data.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "api")
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: i})
		span.SetSpanID(pcommon.SpanID{7: 42})
		span.SetName(fmt.Sprintf("span-%02d", i))
		span.SetStartTimestamp(pcommon.Timestamp(i))
		span.SetEndTimestamp(pcommon.Timestamp(i + 1))
	}
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, data, handler.store.FlushedIDs())
	}))

	result, err := handler.Handle(context.Background(), createRequest("getSpan", map[string]any{
		"spanID": "000000000000002a", "limit": 29,
	}))
	require.NoError(t, err)
	ambiguous := result.(spanAmbiguousResult)
	require.EqualValues(t, 30, ambiguous.MatchCount)
	require.True(t, ambiguous.Truncated)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(ambiguous.Summaries, &summaries))
	require.Len(t, summaries, 29)
	require.Equal(t, "0000000000000000000000000000001e", summaries[0]["traceID"])
	require.Equal(t, "00000000000000000000000000000002", summaries[28]["traceID"])
}

func TestGetSpanNotFoundValidationAndCancellation(t *testing.T) {
	handler := setupHandler(t)
	result, err := handler.Handle(context.Background(), createRequest("getSpan", map[string]any{"spanID": "0000000000000000"}))
	require.NoError(t, err)
	missing := result.(spanNotFoundResult)
	require.Nil(t, missing.TraceID)
	require.Equal(t, "0000000000000000", missing.SpanID)

	for _, params := range []any{
		[]any{}, []any{"0"}, []any{"000000000000000g"}, []any{42}, []any{"0000000000000001", "bad-trace"},
		[]any{"0000000000000001", nil, 0}, []any{"0000000000000001", nil, math.MaxInt64},
		map[string]any{"spanID": "0000000000000001", "unknown": "x"},
	} {
		result, err = handler.Handle(context.Background(), createRequest("getSpan", params))
		require.Nil(t, result)
		require.Error(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = handler.Handle(ctx, createRequest("getSpan", []any{"0000000000000001"}))
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRequestCanceled)
}

func TestGetSpanReturnsFullExactDetailAndOnlyCompositeLogs(t *testing.T) {
	handler := setupHandler(t)
	traceID := pcommon.TraceID{15: 7}
	otherTraceID := pcommon.TraceID{15: 8}
	spanID := pcommon.SpanID{7: 9}
	data := ptrace.NewTraces()
	for _, id := range []pcommon.TraceID{traceID, otherTraceID} {
		rs := data.ResourceSpans().AppendEmpty()
		rs.SetSchemaUrl("resource-schema")
		rs.Resource().SetDroppedAttributesCount(10)
		rs.Resource().Attributes().PutStr("service.name", "exact")
		ss := rs.ScopeSpans().AppendEmpty()
		ss.SetSchemaUrl("scope-schema")
		ss.Scope().SetName("scope")
		ss.Scope().SetVersion("1")
		ss.Scope().SetDroppedAttributesCount(11)
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(id)
		span.SetSpanID(spanID)
		span.SetFlags(0xffffffff)
		span.SetName("exact span")
		span.SetKind(ptrace.SpanKindServer)
		span.SetStartTimestamp(pcommon.Timestamp(^uint64(0) - 10))
		span.SetEndTimestamp(pcommon.Timestamp(^uint64(0)))
		span.SetDroppedAttributesCount(12)
		span.SetDroppedEventsCount(13)
		span.SetDroppedLinksCount(14)
		span.Status().SetCode(ptrace.StatusCodeError)
		span.Attributes().PutInt("max", math.MaxInt64)
		span.Attributes().PutDouble("negative-zero", math.Copysign(0, -1))
		span.Attributes().PutDouble("infinity", math.Inf(1))
		event := span.Events().AppendEmpty()
		event.SetName("event")
		event.SetTimestamp(pcommon.Timestamp(^uint64(0) - 1))
		event.SetDroppedAttributesCount(15)
		link := span.Links().AppendEmpty()
		link.SetFlags(16)
		link.SetDroppedAttributesCount(17)
	}
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, data, handler.store.FlushedIDs())
	}))

	logData := plog.NewLogs()
	for i, id := range []pcommon.TraceID{traceID, otherTraceID} {
		rl := logData.ResourceLogs().AppendEmpty()
		rl.SetSchemaUrl("log-resource-schema")
		rl.Resource().SetDroppedAttributesCount(20)
		sl := rl.ScopeLogs().AppendEmpty()
		sl.SetSchemaUrl("log-scope-schema")
		sl.Scope().SetName("log-scope")
		record := sl.LogRecords().AppendEmpty()
		record.SetTraceID(id)
		record.SetSpanID(spanID)
		record.SetTimestamp(pcommon.Timestamp(^uint64(0) - uint64(i)))
		record.SetObservedTimestamp(pcommon.Timestamp(^uint64(0)))
		record.SetFlags(21)
		record.SetDroppedAttributesCount(22)
		record.Body().SetStr(fmt.Sprintf("trace-%d", id[15]))
	}
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(context.Background(), conn, logData, handler.store.FlushedIDs())
	}))

	result, err := handler.Handle(context.Background(), createRequest("getSpan", map[string]any{
		"spanID": "0000000000000009", "traceID": "00000000000000000000000000000007",
	}))
	require.NoError(t, err)
	found := result.(spanFoundResult)
	encoded, err := json.Marshal(found)
	require.NoError(t, err)
	text := string(encoded)
	for _, want := range []string{
		`"startTime":"18446744073709551605"`, `"endTime":"18446744073709551615"`,
		`"value":"9223372036854775807"`, `"value":"0x8000000000000000"`, `"value":"0x7ff0000000000000"`,
		`"traceID":null`, `"spanID":null`, `"flags":4294967295`, `"kindCode":2`, `"statusCodeValue":2`,
		`"droppedAttributesCount":12`, `"droppedEventsCount":13`, `"droppedLinksCount":14`,
		`"resourceSchemaURL":"resource-schema"`, `"scopeSchemaURL":"scope-schema"`,
		`"resourceSchemaURL":"log-resource-schema"`, `"scopeSchemaURL":"log-scope-schema"`, `"trace-7"`,
	} {
		require.Contains(t, text, want)
	}
	require.NotContains(t, text, "trace-8")
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

func TestGetTraceAttributeDefinitions(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("getTraceAttributeDefinitions", []any{})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		assert.Equal(t, []byte("[]"), []byte(raw), "empty range should return []")
	})

	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		req := createRequest("getTraceAttributeDefinitions", []any{})
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
		handler := setupHandler(t)

		req := createRequest("getTraceAttributeDefinitions", []string{"123"})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})

	t.Run("Invalid Parameter Types", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("getTraceAttributeDefinitions", []string{"pumpkin", "pie"})
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

// TestSearchLogSummariesInvalidParams ensures searchLogSummaries with wrong param count returns ErrInvalidParams.
func TestSearchLogSummariesInvalidParams(t *testing.T) {
	handler := setupHandler(t)

	req := createRequest("searchLogSummaries", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}

// TestSearchMetricSummariesInvalidParams ensures searchMetricSummaries with wrong param count returns ErrInvalidParams.
func TestSearchMetricSummariesInvalidParams(t *testing.T) {
	handler := setupHandler(t)

	req := createRequest("searchMetricSummaries", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
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
}

func TestClearLogs(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("clearLogs", nil))
	assert.NoError(t, err)
	assert.Equal(t, "Logs cleared successfully", result)

	searchResult, err := handler.Handle(ctx, createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &entries))
	assert.Len(t, entries, 0)
}

func TestClearMetrics(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
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
	handler := setupHandlerWithData(t)

	req := createRequest("getLog", []string{"00000000-0000-0000-0000-0000000000aa"})
	result, err := handler.Handle(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrLogsNotFound, err)
}

func TestDeleteSpansByTraceID(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("deleteSpansByTraceID", []string{testTraceIDHex}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	searchResult, err := handler.Handle(ctx, createRequest("searchTraceSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0, "trace should be gone after delete")
}

func TestDeleteLogsByRefs(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	searchResult, err := handler.Handle(ctx, createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 1)
	logRef, ok := entries[0]["logRef"].(string)
	require.True(t, ok)

	result, err := handler.Handle(ctx, createRequest("deleteLogsByRefs", []string{logRef}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	getResult, err := handler.Handle(ctx, createRequest("getLog", []string{logRef}))
	assert.Nil(t, getResult)
	assert.Equal(t, ErrLogsNotFound, err, "deleted log should be gone")
}

func TestDeleteMetric(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
	ctx := context.Background()

	maxNano := strconv.FormatInt(1<<63-1, 10)
	searchResult, err := handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.NotEmpty(t, summaries, "fixture must provide at least one Metric")
	metricRef, ok := summaries[0]["metricRef"].(string)
	require.True(t, ok)
	before := len(summaries)

	result, err := handler.Handle(ctx, createRequest("deleteMetric", []string{metricRef}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric deleted successfully", result)

	// Only the deleted Metric disappears from search.
	searchResult, err = handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok = searchResult.(json.RawMessage)
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, before-1, "exactly one Metric should be gone")
	for _, s := range summaries {
		assert.NotEqual(t, metricRef, s["metricRef"], "deleted Metric must not reappear")
	}
}

// TestDeleteMetricNotFound covers deleting a Metric that does not exist.
// The cascade is a series of unconditional DELETEs, so this is a no-op rather
// than an error -- the UI relies on that when a poll races a delete.
func TestDeleteMetricNotFound(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	result, err := handler.Handle(context.Background(),
		createRequest("deleteMetric", []string{"00000000-0000-0000-0000-0000000000ff"}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric deleted successfully", result)
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

func TestGetLogAttributeDefinitions(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributeDefinitions", []any{}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributeDefinitions", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestGetMetricAttributeDefinitions(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributeDefinitions", []any{}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributeDefinitions", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestGetTraceAttributeDefinitionsByTraceID(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceAttributeDefinitionsByTraceID", []string{testTraceIDHex}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceAttributeDefinitionsByTraceID", []any{42}))
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidTraceID, err)
	})
}

func TestGetTraceSpanCount(t *testing.T) {
	handler := setupHandlerWithData(t)
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

func setupHandlerWithMetrics(t *testing.T) *JSONRPCHandler {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	handler := NewJSONRPCHandler(s, zap.NewNop())
	ctx := context.Background()

	err = s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTestMetrics(), s.FlushedIDs())
	})
	require.NoError(t, err, "ingest metrics")

	return handler
}

func TestSearchMetricSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

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
		handler := setupHandlerWithMetrics(t)

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
		assert.NotEmpty(t, summaries[0]["metricRef"])
		assert.NotNil(t, summaries[0]["seriesCount"])
		assert.NotNil(t, summaries[0]["lastValue"])
	})

	t.Run("With Query", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

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
		handler := setupHandlerWithMetrics(t)
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

func TestGetMetricView(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

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
		metricRef, ok := summaries[0]["metricRef"].(string)
		require.True(t, ok)
		require.NotEmpty(t, metricRef)

		req := createRequest("getMetricView", []any{
			metricRef, "0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		assert.Equal(t, "test.gauge", metric["name"])
		assert.Equal(t, "bytes", metric["unit"])
		// MetricViewData is grouped by timeseries (per attribute set)
		// rather than a flat datapoint list. Each timeseries owns the
		// attributes for its group plus the pure-OTLP datapoints.
		timeseries, _ := metric["timeseries"].([]any)
		require.Len(t, timeseries, 1, "should have one timeseries")
		ts, _ := timeseries[0].(map[string]any)
		require.NotNil(t, ts, "timeseries must be a JSON object")
		assert.Contains(t, ts, "seriesRef", "timeseries should expose its grouping key")
		assert.Contains(t, ts, "attributes", "timeseries should own its attribute set")
		dps, _ := ts["datapoints"].([]any)
		assert.Len(t, dps, 1, "should have one datapoint inside the timeseries")
	})

	t.Run("Not Found", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		req := createRequest("getMetricView", []any{
			"00000000-0000-0000-0000-000000000000",
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrMetricNotFound, err,
			"not-found must use the shared error convention, not a null result")
	})

	// A known Metric queried over a window with no datapoints is NOT a
	// not-found: it returns valid MetricViewData with an empty timeseries list.
	// Only an unknown Metric reference gets ErrMetricNotFound.
	t.Run("Known Metric, Empty Window", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

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
		metricRef, ok := summaries[0]["metricRef"].(string)
		require.True(t, ok)

		// Test data is timestamped time.Now(); the window [0, 1] ns is
		// guaranteed to miss it.
		req := createRequest("getMetricView", []any{metricRef, "0", "1"})
		result, err := handler.Handle(context.Background(), req)

		require.NoError(t, err,
			"an empty window on a known Metric must not be treated as not-found")
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

func TestGetMetricAndSeries(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []any{nil, nil}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.Len(t, summaries, 1)
	metricRef := summaries[0]["metricRef"].(string)

	discoveryResult, err := handler.Handle(context.Background(), createRequest(
		"getMetric", map[string]any{"metricRef": metricRef}))
	require.NoError(t, err)
	var discovery map[string]any
	require.NoError(t, json.Unmarshal(discoveryResult.(json.RawMessage), &discovery))
	require.Equal(t, metricRef, discovery["metricRef"])
	require.Equal(t, "test.gauge", discovery["name"])
	require.Equal(t, "Gauge", discovery["metricType"])
	require.Equal(t, "A test gauge", discovery["description"])
	require.NotContains(t, discovery, "datapoints")
	series := discovery["series"].([]any)
	require.Len(t, series, 1)
	seriesSummary := series[0].(map[string]any)
	require.Equal(t, "1", seriesSummary["datapointCount"])
	require.NotNil(t, seriesSummary["firstDatapointTimestamp"])
	require.Equal(t, seriesSummary["firstDatapointTimestamp"], seriesSummary["lastDatapointTimestamp"])

	seriesResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", map[string]any{
			"metricRef": metricRef, "seriesRef": seriesSummary["seriesRef"],
			"startTime": nil, "endTime": nil,
		}))
	require.NoError(t, err)
	var selected map[string]any
	require.NoError(t, json.Unmarshal(seriesResult.(json.RawMessage), &selected))
	require.Equal(t, metricRef, selected["metricRef"])
	require.Equal(t, seriesSummary["seriesRef"], selected["seriesRef"])
	require.Len(t, selected["datapoints"].([]any), 1)

	emptyResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", []any{metricRef, seriesSummary["seriesRef"], "0", "1"}))
	require.NoError(t, err)
	var empty map[string]any
	require.NoError(t, json.Unmarshal(emptyResult.(json.RawMessage), &empty))
	require.Empty(t, empty["datapoints"])

	missingResult, err := handler.Handle(context.Background(), createRequest(
		"getMetric", []any{"00000000-0000-0000-0000-000000000000"}))
	require.Nil(t, missingResult)
	require.Equal(t, ErrMetricNotFound, err)
	missingSeriesResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", []any{metricRef, "00000000-0000-0000-0000-000000000000", nil, nil}))
	require.Nil(t, missingSeriesResult)
	require.Equal(t, ErrMetricNotFound, err)
}

func TestMetricHandlersAcceptNullableBoundsPositionallyAndByName(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []any{nil, nil}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.Len(t, summaries, 1)
	metricRef := summaries[0]["metricRef"].(string)

	for _, tc := range rpcNullableRangeCases() {
		t.Run("getMetric/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetricView", metricRangeParams(metricRef, tc)))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &got))
			require.Equal(t, "test.gauge", got["name"])
			require.Equal(t, float64(1), got["datapointCount"])

			lastSeen := got["lastSeenNs"].(string)
			window := got["window"].(map[string]any)
			requested := window["requested"].(map[string]any)
			effective := window["effective"].(map[string]any)
			require.Equal(t, map[string]any{"startNs": tc.start, "endNs": tc.end}, requested)
			wantEffectiveStart := tc.start
			if wantEffectiveStart == nil {
				wantEffectiveStart = lastSeen
			}
			wantEffectiveEnd := tc.end
			if wantEffectiveEnd == nil {
				wantEffectiveEnd = lastSeen
			}
			require.Equal(t, map[string]any{
				"startNs": wantEffectiveStart, "endNs": wantEffectiveEnd,
			}, effective)
		})

		t.Run("getMetricAggregateView/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetricAggregateView", metricRangeParams(metricRef, tc)))
			require.NoError(t, err)
			require.JSONEq(t,
				`{"aggregate":null,"scalarAggregate":{"selected":[],"all":[]}}`,
				string(result.(json.RawMessage)))
		})
	}
}

// searchAttributeMatches is the value-first counterpart to the getXAttributes
// discovery methods: given text seen in the UI, which keys hold it. It takes no
// time range and no signal, because the dictionary it reads is shared by all
// three.
func TestSearchAttributeMatches(t *testing.T) {
	call := func(t *testing.T, handler *JSONRPCHandler, params any) []map[string]any {
		t.Helper()
		result, err := handler.Handle(context.Background(), createRequest("searchAttributeMatches", params))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "expected json.RawMessage, got %T", result)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("finds the key holding a value", func(t *testing.T) {
		handler := setupHandlerWithData(t)

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
		assert.Contains(t, found["sampleValues"], map[string]any{"kind": "string", "value": "pumpkin.pie"})
	})

	t.Run("no match and empty term return an empty list", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		assert.Empty(t, call(t, handler, []string{"no-such-text-anywhere"}))
		assert.Empty(t, call(t, handler, []string{""}),
			"an empty term is not a request for the whole dictionary")
	})

	t.Run("rejects malformed params", func(t *testing.T) {
		handler := setupHandler(t)

		for _, params := range []any{[]string{}, []string{"a", "b"}, []int{1}} {
			_, err := handler.Handle(context.Background(), createRequest("searchAttributeMatches", params))
			assert.Error(t, err, "params %v", params)
		}
	})
}

// TestMetricHandlersAcceptEveryParameter pins both contiguous positional
// contracts and their distinct final arities.
func TestMetricHandlersAcceptEveryParameter(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.NotEmpty(t, summaries)
	metricRef := summaries[0]["metricRef"].(string)
	maxTime := strconv.FormatInt(1<<63-1, 10)

	detail := []any{
		metricRef,        // 1 Metric
		"0",              // 2 start
		maxTime,          // 3 end
		"100",            // 4 targetBuckets
		[]any{},          // 5 seriesRefs
		[]any{0.5, 0.95}, // 6 quantiles
		"0",              // 7 tzOffsetNs
		"120",            // 8 viewBuckets
		"64",             // 9 sparklineBuckets
		[]any{},          // 10 selectedSeriesRefs
		"Europe/London",  // 11 tzName
		[]any{},          // 12 datapointSeriesRefs
		"10",             // 13 datapointSeriesLimit
	}
	aggregate := []any{
		metricRef,        // 1 Metric
		"0",              // 2 start
		maxTime,          // 3 end
		"100",            // 4 targetBuckets
		[]any{},          // 5 seriesRefs
		[]any{0.5, 0.95}, // 6 quantiles
		"0",              // 7 tzOffsetNs
		"120",            // 8 viewBuckets
		[]any{},          // 9 selectedSeriesRefs
		"Europe/London",  // 10 tzName
	}

	for method, full := range map[string][]any{
		"getMetricView": detail, "getMetricAggregateView": aggregate,
	} {
		t.Run(method, func(t *testing.T) {
			for n := 3; n <= len(full); n++ {
				t.Run(fmt.Sprintf("%d params", n), func(t *testing.T) {
					result, err := handler.Handle(context.Background(),
						createRequest(method, full[:n]))
					require.NoErrorf(t, err, "%s with %d parameters", method, n)
					require.NotNil(t, result)
				})
			}

			_, err := handler.Handle(context.Background(),
				createRequest(method, append(append([]any{}, full...), "extra")))
			assert.Error(t, err, "a parameter beyond the known list must be refused")
		})
	}
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

// getFieldValueCompletions is the search box's value-completion source. Its guards are
// what keep an allowlist an allowlist, so they are pinned rather than left to
// the named-params table walk, which only checks the method has a name list.
func TestGetFieldValueCompletions(t *testing.T) {
	handler := setupHandlerWithData(t)
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
			createRequest("getFieldValueCompletions", []any{"traces", "name", "", 10}))
		require.NoError(t, err)
		assert.NotEmpty(t, decode(t, result))
	})

	t.Run("named params reach the same place", func(t *testing.T) {
		result, err := handler.Handle(ctx, createRequest("getFieldValueCompletions",
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
			createRequest("getFieldValueCompletions", []any{"traces", "name", "", 100000}))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(decode(t, result)), 500)
	})

	t.Run("clamps a limit below one", func(t *testing.T) {
		for _, limit := range []any{0, -5} {
			result, err := handler.Handle(ctx,
				createRequest("getFieldValueCompletions", []any{"traces", "name", "", limit}))
			require.NoError(t, err, "limit %v", limit)
			assert.Len(t, decode(t, result), 1, "limit %v", limit)
		}
	})

	t.Run("refuses a field outside the allowlist", func(t *testing.T) {
		// statusMessage is a real span column, deliberately not completable:
		// the allowlist is a boundary, not a convenience.
		_, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"traces", "statusMessage", "", 10}))
		require.Error(t, err)
	})

	t.Run("refuses an unknown signal", func(t *testing.T) {
		_, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"spans", "name", "", 10}))
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
				_, err := handler.Handle(ctx, createRequest("getFieldValueCompletions", tc.params))
				assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
			})
		}
	})
}
