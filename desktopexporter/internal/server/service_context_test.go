package server

import (
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

func TestServicesGroupResourcesWithoutSplittingScopesOrDoubleCountingTraces(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for resourceIndex := range 2 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", "shared")
		r.Resource().Attributes().PutInt("unrelated", int64(resourceIndex))
		if resourceIndex == 1 {
			r.Resource().Attributes().PutStr("service.namespace", "")
		}
		for scopeIndex := range 2 {
			scope := r.ScopeSpans().AppendEmpty()
			scope.Scope().SetName(fmt.Sprint(scopeIndex))
			// Scope-owned service-looking keys must not become Resource identity.
			scope.Scope().Attributes().PutStr("service.name", fmt.Sprint(scopeIndex))
			for i := range 100 {
				span := scope.Spans().AppendEmpty()
				span.SetTraceID(pcommon.TraceID{15: 1})
				id := resourceIndex*200 + scopeIndex*100 + i + 1
				span.SetSpanID(pcommon.SpanID{6: byte(id >> 8), 7: byte(id)})
				span.SetStartTimestamp(pcommon.Timestamp(i))
				span.SetEndTimestamp(pcommon.Timestamp(i + 1))
			}
		}
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	for _, window := range []bool{false, true} {
		options := map[string]any{"kind": "services", "queries": map[string]any{}}
		want := 400
		if window {
			options["startTime"], options["endTime"] = "10", "19"
			want = 40
		}
		raw, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var page struct {
			Rows [][]struct {
				Spans  int `json:"spanCount"`
				Traces int `json:"traceCount"`
			} `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(raw.(json.RawMessage), &page))
		require.Len(t, page.Rows, 1)
		require.Equal(t, want, page.Rows[0][0].Spans)
		require.Equal(t, 1, page.Rows[0][0].Traces)
	}
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	// Reads aggregate compact resource IDs. The catalogue holds the typed pair
	// once despite unrelated attributes, scopes and hundreds of owning records.
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var key, raw string
		err := db.QueryRowContext(t.Context(), "EXPLAIN (ANALYZE, FORMAT JSON) "+discovery.Services(discovery.Query{})).Scan(&key, &raw)
		if err != nil {
			return err
		}
		require.Contains(t, raw, "search_service_resources")
		var services, resources int
		require.NoError(t, db.QueryRow(`select (select count(*) from search_services),(select count(*) from search_service_resources)`).Scan(&services, &resources))
		require.Equal(t, 1, services)
		require.Equal(t, 2, resources)
		require.NotContains(t, raw, "otel_native_text")
		return nil
	}))
}

func TestServiceCountIsScalarBeyondOneSummaryPage(t *testing.T) {
	h := setupHandler(t)
	empty, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 0, empty)
	data := ptrace.NewTraces()
	for i := range 105 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", fmt.Sprint(i))
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 105, count)
	for _, params := range []any{[]any{1}, map[string]any{"limit": 1}} {
		_, err := h.Handle(t.Context(), createRequest("getServiceCount", params))
		require.Error(t, err)
	}
}

func TestServiceCatalogueBoundsLabelsButKeepsExactSelectionAndCopy(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	prefix := strings.Repeat("雪'quoted\"\\", 2000)
	data := ptrace.NewTraces()
	for i, suffix := range []string{"z-tail", "a-tail"} {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", prefix+suffix)
		r.Resource().Attributes().PutStr("service.namespace", prefix)
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{7: 1})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	read := func(options string) *array.Struct {
		record := readArrowDataset(t, s, "discoverTelemetry", `{"options":`+options+`}`)
		return record.Column(0).(*array.List).ListValues().(*array.Struct)
	}
	field := func(items *array.Struct, name string) *array.String {
		index, ok := items.DataType().(*arrow.StructType).FieldIdx(name)
		require.True(t, ok)
		return items.Field(index).(*array.String)
	}
	items := read(`{"kind":"services","limit":1}`)
	require.Equal(t, 1, items.Len())
	ref := field(items, "serviceIdentity").Value(0)
	require.Len(t, ref, 64)
	require.LessOrEqual(t, len([]rune(field(items, "serviceName").Value(0))), 300)
	require.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(prefix+"a-tail"))), field(items, "serviceColorKey").Value(0))
	labels, err := h.Handle(t.Context(), createRequest("getServiceLabels", map[string]any{"serviceRef": ref}))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"serviceName": prefix + "a-tail", "serviceNamespace": prefix}, labels)
	second := read(`{"kind":"services","offset":1,"limit":1}`)
	require.NotEqual(t, ref, field(second, "serviceIdentity").Value(0))
	require.Equal(t, field(items, "serviceName").Value(0), field(second, "serviceName").Value(0))
	filtered := read(`{"kind":"services","term":"a-tail"}`)
	require.Equal(t, 1, filtered.Len())
	require.Equal(t, ref, field(filtered, "serviceIdentity").Value(0))
	selected := read(`{"kind":"services","serviceIdentity":"` + ref + `"}`)
	require.Equal(t, 1, selected.Len())
	_, err = h.Handle(t.Context(), createRequest("deleteSpansByTraceID", []any{"00000000000000000000000000000002"}))
	require.NoError(t, err)
	_, err = h.Handle(t.Context(), createRequest("getServiceLabels", map[string]any{"serviceRef": ref}))
	require.ErrorIs(t, err, ErrHomeValueUnavailable)
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestServiceCatalogueCanonicalNaNsAcrossDistinctResources(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for i, bits := range []uint64{0x7ff8000000000001, 0x7ff8000000000002, 0xfff8000000000001} {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutDouble("service.name", math.Float64frombits(bits))
		r.Resource().Attributes().PutEmptyMap("service.namespace").PutDouble("nested", math.Float64frombits(bits))
		r.Resource().Attributes().PutInt("unrelated", int64(i))
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var services, resources int
		require.NoError(t, db.QueryRow(`select (select count(*) from search_services),(select count(*) from search_service_resources)`).Scan(&services, &resources))
		require.Equal(t, 1, services)
		require.Equal(t, 3, resources)
		return nil
	}))
}
