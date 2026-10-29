package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/apache/arrow-go/v18/arrow/array"
	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

func homePage(t *testing.T, h *JSONRPCHandler, options map[string]any) *homeValuePage {
	t.Helper()
	options["kind"] = "home-values"
	result, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": options}))
	require.NoError(t, err)
	return result.(*homeValuePage)
}

func TestHomeDiscoveryGlobalPageAndExactReference(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	data := ptrace.NewTraces()
	scope := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i := range 64 {
		span := scope.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
		span.SetStartTimestamp(100)
		span.SetEndTimestamp(101)
		span.Attributes().PutInt("value", int64(i))
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	options := map[string]any{"ownerType": "span", "key": "value", "valueKind": "int64", "limit": 25, "sortBy": "name", "sortDirection": "asc"}
	page := homePage(t, h, options)
	require.Len(t, page.Values, 25)
	require.EqualValues(t, 64, page.Total)
	require.EqualValues(t, 64, page.Denominator)
	require.True(t, page.Truncated)
	require.Equal(t, "0", page.Values[0].Preview)
	require.Equal(t, "1", page.Values[1].Preview)
	require.Equal(t, "10", page.Values[2].Preview) // Full name collation, not numeric sort.
	first := page.Values[0].Identity
	require.True(t, strings.HasPrefix(first, "home:"))
	require.Equal(t, first, homePage(t, h, options).Values[0].Identity)
	options["offset"] = 25
	second := homePage(t, h, options)
	require.Len(t, second.Values, 25)
	for _, a := range page.Values {
		for _, b := range second.Values {
			require.NotEqual(t, a.Identity, b.Identity)
		}
	}
	options["offset"] = 50
	last := homePage(t, h, options)
	require.Len(t, last.Values, 14)
	require.False(t, last.Truncated)
	options["offset"] = 100
	require.Empty(t, homePage(t, h, options).Values)

	// Copy resolves the retained received value after its source was removed.
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error { _, err := db.ExecContext(t.Context(), "DELETE FROM spans"); return err }))
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	// The HTTP server has its own handler/cache; use the discovery owner here.
	s.jsonrpcHandler = h
	wire, err := json.Marshal(map[string]any{"options": map[string]any{"kind": "home-value", "valueRef": first}})
	require.NoError(t, err)
	resolved := readArrowDataset(t, s, "discoverTelemetry", string(wire))
	require.Equal(t, `{"kind":"int64","value":"0"}`, resolved.Column(1).(*array.String).Value(0))
	_, err = setupHandler(t).homeValueArrow(t.Context(), discoveryOptions{Kind: "home-value", ValueRef: first})
	require.Error(t, err)
	require.Contains(t, err.Error(), "refresh attribute values")
}

func TestHomeDiscoveryCrossSignalCountsAndBoundedOutput(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	ctx := context.Background()
	traces, logData, metricData := buildTestTraces(), buildTestLogs(), buildTestMetrics()
	for _, resource := range []pcommon.Resource{traces.ResourceSpans().At(0).Resource(), logData.ResourceLogs().At(0).Resource(), metricData.ResourceMetrics().At(0).Resource()} {
		resource.Attributes().PutStr("service.name", "shared")
		require.NoError(t, resource.Attributes().PutEmpty("deep").FromRaw(map[string]any{"full": strings.Repeat("exact value 雪 ", 3000), "int": int64(9007199254740993)}))
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		if err := spans.Ingest(ctx, conn, traces); err != nil {
			return err
		}
		if err := logs.Ingest(ctx, conn, logData); err != nil {
			return err
		}
		return metrics.Ingest(ctx, conn, metricData)
	}))
	options := map[string]any{"ownerType": "resource", "key": "deep", "valueKind": "map", "serviceName": "shared"}
	page := homePage(t, h, options)
	require.Len(t, page.Values, 1)
	item := page.Values[0]
	require.Positive(t, item.Counts.Traces)
	require.Positive(t, item.Counts.Logs)
	require.Positive(t, item.Counts.Metrics)
	require.Equal(t, page.Denominators, item.Counts)
	require.Equal(t, page.Denominator, item.Count)
	require.Equal(t, float64(1), item.RelativeFrequency)
	require.Len(t, item.FoundOn, 3)
	wire, err := json.Marshal(page)
	require.NoError(t, err)
	require.Less(t, len(wire), 3000)
	require.NotContains(t, string(wire), strings.Repeat("exact value", 2))
	options["queries"] = map[string]any{"logs": false, "metrics": false}
	filtered := homePage(t, h, options)
	require.EqualValues(t, 0, filtered.Denominators.Logs)
	require.EqualValues(t, 0, filtered.Denominators.Metrics)
	require.Equal(t, item.ColorHash, filtered.Values[0].ColorHash)
	options["term"] = "NOT IN PREVIEW"
	require.Empty(t, homePage(t, h, options).Values)
	options["term"] = "9007199254740993" // Exact full content is searchable even when omitted from preview.
	require.Len(t, homePage(t, h, options).Values, 1)
}

func TestHomeDiscoveryCanonicalNaNsAndReceivedRepresentative(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	data := ptrace.NewTraces()
	scope := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i, value := range []float64{math.Float64frombits(0x7ff8000000000001), math.Float64frombits(0x7ff8000000000002), math.Copysign(0, -1), 0, math.Inf(1), math.Inf(-1)} {
		span := scope.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
		span.SetStartTimestamp(100)
		span.SetEndTimestamp(101)
		span.Attributes().PutDouble("number", value)
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	page := homePage(t, h, map[string]any{"ownerType": "span", "key": "number", "sortBy": "frequency", "sortDirection": "desc"})
	require.Len(t, page.Values, 5)
	require.EqualValues(t, 2, page.Values[0].Count)
	var entry *homeReference
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var err error
		entry, err = h.homeValues.resolve(db, page.Values[0].Identity)
		return err
	}))
	require.Contains(t, entry.key.identity, "7ff8000000000000")
	native, err := parquetvariant.New(entry.metadata, entry.value)
	require.NoError(t, err)
	require.Equal(t, uint64(0x7ff8000000000001), math.Float64bits(native.Value().(float64)))
	result, err := h.homeValueArrow(t.Context(), discoveryOptions{Kind: "home-value", ValueRef: page.Values[0].Identity})
	require.NoError(t, err)
	result.Close()
}

func TestHomeDiscoverySummaryAndCancellation(t *testing.T) {
	h := setupHandler(t)
	data := buildTestTraces()
	data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes().PutStr("http.method", "GET")
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	page := homePage(t, h, map[string]any{"ownerType": "span", "key": "http.method", "limit": 0})
	require.Empty(t, page.Values)
	require.Positive(t, page.Total)
	require.Positive(t, page.Denominator)
	require.Empty(t, h.homeValues.byRef)
	excluded := homePage(t, h, map[string]any{"ownerType": "span", "key": "http.method", "queries": map[string]any{"traces": false, "logs": false, "metrics": false}})
	require.Empty(t, excluded.Values)
	require.Zero(t, excluded.Total)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "home-values", "ownerType": "span", "key": "http.method"}}))
	require.ErrorIs(t, err, ErrRequestCanceled)
}

func TestHomeReferenceEvictionNeverReusesTokens(t *testing.T) {
	var cache homeValueReferences
	db := new(sql.DB)
	first, _, err := cache.retain(db, "same", []byte{1}, []byte{2})
	require.NoError(t, err)
	for i := range homeReferenceEntries {
		_, _, err = cache.retain(db, strconv.Itoa(i), []byte{1}, []byte{2})
		require.NoError(t, err)
	}
	_, err = cache.resolve(db, first)
	require.Error(t, err)
	next, _, err := cache.retain(db, "same", []byte{1}, []byte{2})
	require.NoError(t, err)
	require.NotEqual(t, first, next)
}

func TestHomeReferenceSeparatesCanonicalRowsFromReceivedRepresentatives(t *testing.T) {
	var cache homeValueReferences
	db := new(sql.DB)
	first, nativeA, err := cache.retain(db, "canonical NaN", []byte{1}, []byte{2})
	require.NoError(t, err)
	next, nativeB, err := cache.retain(db, "canonical NaN", []byte{1}, []byte{3})
	require.NoError(t, err)
	require.Equal(t, first, next)
	require.NotEqual(t, nativeA, nativeB)
	resolved, err := cache.resolve(db, nativeB)
	require.NoError(t, err)
	require.Equal(t, []byte{3}, resolved.value)
	_, err = cache.resolve(new(sql.DB), nativeB)
	require.ErrorIs(t, err, ErrHomeValueUnavailable)
}

func TestHomeNameSortPreservesStableCanonicalCollationTies(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	scope := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i, text := range []string{"é", "é", "e\u0301"} {
		span := scope.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15:1}); span.SetSpanID(pcommon.SpanID{7:byte(i+1)})
		span.SetStartTimestamp(100); span.SetEndTimestamp(101)
		span.Attributes().PutStr("letter",text)
	}
	require.NoError(t,h.store.WithConn(func(conn driver.Conn) error{return spans.Ingest(t.Context(),conn,data)}))
	for _, direction := range []string{"asc","desc"} {
		page:=homePage(t,h,map[string]any{"ownerType":"span","key":"letter","sortBy":"name","sortDirection":direction,"locale":"en-US"})
		require.Len(t,page.Values,2)
		// Both Copy text and canonical identity collate equally. The original
		// stable frontend sort retained the source's descending-frequency order.
		require.Equal(t,"é",page.Values[0].Preview)
		require.EqualValues(t,2,page.Values[0].Count)
	}
}

func TestServiceNativePairsKeepDistinctIDsAcrossCanonicalNaNs(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for i := range 2 {
		resource := data.ResourceSpans().AppendEmpty()
		resource.Resource().Attributes().PutDouble("service.name", math.Float64frombits(0x7ff8000000000001+uint64(i)))
		span:=resource.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15:1});span.SetSpanID(pcommon.SpanID{7:byte(i+1)})
		span.SetStartTimestamp(100);span.SetEndTimestamp(101)
		md:=buildTestMetrics()
		md.ResourceMetrics().At(0).Resource().Attributes().PutDouble("service.name",math.Float64frombits(0x7ff8000000000001+uint64(i)))
		md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).SetTimestamp(pcommon.Timestamp(100+i))
		require.NoError(t,h.store.WithConn(func(conn driver.Conn) error{return metrics.Ingest(t.Context(),conn,md)}))
	}
	require.NoError(t,h.store.WithConn(func(conn driver.Conn) error{return spans.Ingest(t.Context(),conn,data)}))
	result,err:=h.Handle(t.Context(),createRequest("discoverTelemetry",map[string]any{"options":map[string]any{"kind":"services"}}))
	require.NoError(t,err)
	var page struct { Rows [][]struct{TraceCount, SpanCount, MetricCount, DataPointCount uint64} }
	require.NoError(t,json.Unmarshal(result.(json.RawMessage),&page))
	require.Len(t,page.Rows,1)
	counts:=page.Rows[0][0]
	require.EqualValues(t,1,counts.TraceCount)
	require.EqualValues(t,2,counts.SpanCount)
	require.EqualValues(t,1,counts.MetricCount)
	require.EqualValues(t,2,counts.DataPointCount)
}
