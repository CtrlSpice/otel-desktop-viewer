package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"testing"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

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

func TestGetTraceView(t *testing.T) {
	handler := setupHandlerWithData(t)
	t.Run("Selected spans", func(t *testing.T) {
		result, err := handler.Handle(context.Background(), createRequest("getTraceView", map[string]any{
			"traceID": testTraceIDHex, "spanIDs": []string{"ffffffffffffffff"},
		}))
		require.NoError(t, err)
		var trace map[string]any
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &trace))
		require.Empty(t, trace["spans"])
	})
	t.Run("Reject invalid selections", func(t *testing.T) {
		for _, ids := range []any{[]string{}, []string{"bad"}, "0000000000000001", make([]string, 257)} {
			_, err := handler.Handle(context.Background(), createRequest("getTraceView", map[string]any{"traceID": testTraceIDHex, "spanIDs": ids}))
			require.Error(t, err)
		}
	})

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
		return spans.Ingest(context.Background(), conn, traces)
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
		return spans.Ingest(context.Background(), conn, data)
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
		return spans.Ingest(context.Background(), conn, data)
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
		return spans.Ingest(context.Background(), conn, data)
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
		return logs.Ingest(context.Background(), conn, logData)
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
