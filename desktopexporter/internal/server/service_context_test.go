package server

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
	// Inspect actual optimizer cardinalities: 400 owner rows reduce to two
	// resource contexts before the presence/extraction projections are evaluated.
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var key, raw string
		err := db.QueryRowContext(t.Context(), "EXPLAIN (ANALYZE, FORMAT JSON) "+discovery.Services(discovery.Query{})).Scan(&key, &raw)
		if err != nil {
			return err
		}
		var profile struct {
			Operators []servicePlanNode `json:"operator"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &profile))
		found := false
		var walk func(servicePlanNode)
		walk = func(node servicePlanNode) {
			if node.Info.CTEName == "service_fields" {
				require.NotEmpty(t, node.Children)
				require.EqualValues(t, 2, node.Children[0].Rows)
				found = true
			}
			for _, child := range node.Children {
				walk(child)
			}
		}
		for _, node := range profile.Operators {
			walk(node)
		}
		require.True(t, found, "service field extraction must have a bounded shared stage")
		return nil
	}))
}

type servicePlanNode struct {
	Rows int `json:"intermediate_rows"`
	Info struct {
		CTEName string `json:"CTE Name"`
	} `json:"extra_info"`
	Children []servicePlanNode `json:"children"`
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
