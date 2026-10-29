package server

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

func traceStructField(t *testing.T, value *array.Struct, name string) arrow.Array {
	t.Helper()
	index, ok := value.DataType().(*arrow.StructType).FieldIdx(name)
	require.True(t, ok, name)
	return value.Field(index)
}

func traceRows(t *testing.T, record arrow.RecordBatch) *array.Struct {
	t.Helper()
	return record.Column(record.Schema().FieldIndices("spans")[0]).(*array.List).ListValues().(*array.Struct)
}

func TestTraceWaterfallAndExactDetails(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for i := byte(1); i <= 2; i++ {
		r := data.ResourceSpans().AppendEmpty()
		r.SetSchemaUrl("resource-schema")
		r.Resource().Attributes().PutStr("service.name", "trace-test")
		r.Resource().Attributes().PutStr("unselected.resource", "resource")
		scope := r.ScopeSpans().AppendEmpty()
		scope.SetSchemaUrl("scope-schema")
		scope.Scope().Attributes().PutStr("unselected.scope", "scope")
		s := scope.Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{15: i})
		s.SetSpanID(pcommon.SpanID{7: 1})
		s.SetName(strings.Repeat(string('a'+i), 8))
		s.SetStartTimestamp(100)
		s.SetEndTimestamp(200)
		s.Attributes().PutStr("unselected.span", "span")
		s.Attributes().PutInt("exact.integer", 9007199254740993)
		items := s.Attributes().PutEmptySlice("deep")
		for j := 0; j < 256; j++ {
			items.AppendEmpty().SetStr(strings.Repeat("large", 500))
		}
		e := s.Events().AppendEmpty()
		e.SetName("tool.execution")
		e.SetTimestamp(123)
		e.Attributes().PutStr("gen_ai.tool.name", "shell")
		e.Attributes().PutBool("tool.is_error", true)
		e.Attributes().PutStr("opencode.event.timestamp.source", "received")
		e.Attributes().PutStr("unselected.event", strings.Repeat("event", 500))
		s.Links().AppendEmpty().Attributes().PutStr("link.detail", "complete")
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	trace := readArrowDataset(t, s, "getTraceView", `{"traceID":"`+testTraceIDHex+`","columns":[{"source":"span","key":"deep"}]}`)
	rows := traceRows(t, trace)
	require.Equal(t, 1, rows.Len())
	span := traceStructField(t, rows, "spanData").(*array.Struct)
	spanType := span.DataType().(*arrow.StructType)
	_, hasAttributes := spanType.FieldIdx("attributes")
	_, hasLinks := spanType.FieldIdx("links")
	require.False(t, hasAttributes)
	require.False(t, hasLinks)
	columns := trace.Column(trace.Schema().FieldIndices("availableColumns")[0]).(*array.List).ListValues().(*array.Struct)
	keys := traceStructField(t, columns, "key").(*array.String)
	var available []string
	for i := 0; i < keys.Len(); i++ {
		available = append(available, keys.Value(i))
	}
	require.Subset(t, available, []string{"unselected.span", "unselected.resource", "unselected.scope", "deep"})
	events := traceStructField(t, span, "events").(*array.List).ListValues().(*array.Struct)
	require.Equal(t, int64(123), int64(traceStructField(t, events, "timestamp").(*array.Uint64).Value(0)))
	_, hasEventAttributes := events.DataType().(*arrow.StructType).FieldIdx("attributes")
	require.False(t, hasEventAttributes)
	summary := traceStructField(t, events, "summaryAttributes").(*array.Map)
	summaryKeys := summary.Keys().(*array.String)
	for key, value := range map[string]string{"gen_ai.tool.name": "shell", "tool.is_error": "true", "opencode.event.timestamp.source": "received"} {
		found := false
		for i := 0; i < summaryKeys.Len(); i++ {
			if summaryKeys.Value(i) == key {
				require.Contains(t, summary.Items().ValueStr(i), value)
				found = true
			}
		}
		require.True(t, found, key)
	}
	previews := readArrowDataset(t, s, "getTraceColumnPreviews", `{"traceID":"`+testTraceIDHex+`","columns":[{"source":"span","key":"deep"},{"source":"span","key":"missing"}]}`)
	require.Equal(t, int64(2), previews.NumRows())
	values := previews.Column(previews.Schema().FieldIndices("values")[0]).(*array.List)
	texts := traceStructField(t, values.ListValues().(*array.Struct), "preview").(*array.String)
	require.Contains(t, texts.Value(0), "+253 items")
	require.Less(t, len(texts.Value(0)), 500)
	start, end := values.ValueOffsets(1)
	require.Equal(t, start, end, "missing is not received null")
	for _, id := range []string{testTraceIDHex, "00000000000000000000000000000002"} {
		params, err := json.Marshal(map[string]any{"traceID": id, "spanID": "0000000000000001"})
		require.NoError(t, err)
		detail := readArrowDataset(t, s, "getSpan", string(params))
		require.Equal(t, id, detail.Column(detail.Schema().FieldIndices("traceID")[0]).(*array.String).Value(0))
		full := traceStructField(t, traceRows(t, detail), "spanData").(*array.Struct)
		require.Equal(t, 1, traceStructField(t, full, "links").(*array.List).ListValues().Len())
		attrs := traceStructField(t, full, "attributes").(*array.Map)
		require.GreaterOrEqual(t, attrs.ListValues().Len(), 2)
		fullEvents := traceStructField(t, full, "events").(*array.List).ListValues().(*array.Struct)
		_, complete := fullEvents.DataType().(*arrow.StructType).FieldIdx("attributes")
		require.True(t, complete)
	}
	var exported json.RawMessage
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var err error
		exported, err = spans.GetTraceSelectionOTLP(t.Context(), db, testTraceIDHex, "1")
		return err
	}))
	require.Greater(t, len(exported), 500000)
	for _, exact := range []string{strings.Repeat("large", 500), "9007199254740993", "unselected.event", "link.detail", "resource-schema", "scope-schema"} {
		require.Contains(t, string(exported), exact)
	}
	require.NotContains(t, string(exported), "summaryAttributes")
	require.NotContains(t, string(exported), "… (+")
	added := ptrace.NewTraces()
	r := added.ResourceSpans().AppendEmpty()
	r.Resource().Attributes().PutStr("new.resource.key", "received later")
	scope := r.ScopeSpans().AppendEmpty()
	scope.Scope().Attributes().PutStr("new.scope.key", "received later")
	child := scope.Spans().AppendEmpty()
	child.SetTraceID(pcommon.TraceID{15: 1})
	child.SetSpanID(pcommon.SpanID{7: 2})
	child.SetParentSpanID(pcommon.SpanID{7: 1})
	child.SetStartTimestamp(150)
	child.SetEndTimestamp(175)
	child.Attributes().PutStr("new.span.key", "received later")
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, added) }))
	refreshed := readArrowDataset(t, s, "getTraceView", `{"traceID":"`+testTraceIDHex+`"}`)
	require.Equal(t, 2, traceRows(t, refreshed).Len())
	catalogue := refreshed.Column(refreshed.Schema().FieldIndices("availableColumns")[0]).(*array.List).ListValues().(*array.Struct)
	newKeys := traceStructField(t, catalogue, "key").(*array.String)
	var retained []string
	for i := 0; i < newKeys.Len(); i++ {
		retained = append(retained, newKeys.Value(i))
	}
	require.Subset(t, retained, []string{"deep", "unselected.resource", "new.resource.key", "new.scope.key", "new.span.key"})
	missing, err := h.readArrow(t.Context(), "getSpan", json.RawMessage(`{"traceID":"00000000000000000000000000000002","spanID":"0000000000000002"}`))
	require.NoError(t, err)
	defer missing.Close()
	require.Zero(t, missing.NumRows(), "a matching span ID in another trace is not this record")
}
