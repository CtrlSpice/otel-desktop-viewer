package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

func TestServiceMapConnections(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	ctx := context.Background()
	data := ptrace.NewTraces()
	add := func(service, namespace string, trace, id, parent byte, start uint64) ptrace.Span {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", service)
		r.Resource().Attributes().PutStr("service.namespace", namespace)
		s := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{trace})
		s.SetSpanID(pcommon.SpanID{id})
		if parent != 0 {
			s.SetParentSpanID(pcommon.SpanID{parent})
		}
		s.SetName(service)
		s.SetStartTimestamp(pcommon.Timestamp(start))
		s.SetEndTimestamp(pcommon.Timestamp(start + 10))
		return s
	}
	add("api", "one", 1, 1, 0, 100)
	add("worker", "", 1, 2, 1, 110)
	add("worker", "", 1, 3, 1, 120).Status().SetCode(ptrace.StatusCodeError)
	add("api", "one", 1, 4, 2, 130) // reverse edge, not a duplicate node
	add("api", "two", 1, 5, 1, 140) // same name, distinct namespace
	add("orphan", "", 2, 6, 1, 150) // parent ID in another trace is not a connection
	add("api", "one", 1, 7, 1, 160) // same-service span is not an edge
	add("api", "one", 3, 1, 0, 90)
	add("worker", "", 3, 2, 1, 95) // same edge in a second trace
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
	identity := `[{"kind":"string","value":"api"},{"kind":"string","value":"one"}]`
	read := func(options map[string]any) struct {
		Rows      [][]json.RawMessage `json:"rows"`
		Truncated bool                `json:"truncated"`
	} {
		t.Helper()
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var page struct {
			Rows      [][]json.RawMessage `json:"rows"`
			Truncated bool                `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
		return page
	}
	options := map[string]any{"kind": "connections", "serviceIdentity": identity, "limit": 1}
	page := read(options)
	require.Len(t, page.Rows, 1)
	require.True(t, page.Truncated)
	options["limit"] = 100
	page = read(options)
	require.Len(t, page.Rows, 3)
	options["offset"] = 1
	require.Equal(t, page.Rows[1:], read(options).Rows)
	delete(options, "offset")
	var workerIdentity string
	for _, row := range page.Rows {
		var edge struct {
			Source struct {
				Identity string `json:"serviceIdentity"`
			} `json:"source"`
			Target struct {
				Name     string `json:"serviceName"`
				Identity string `json:"serviceIdentity"`
			} `json:"target"`
			SpanCount      int `json:"spanCount"`
			TraceCount     int `json:"traceCount"`
			ErrorSpanCount int `json:"errorSpanCount"`
		}
		require.NoError(t, json.Unmarshal(row[0], &edge))
		if edge.Source.Identity == identity && edge.Target.Name == "worker" {
			workerIdentity = edge.Target.Identity
			require.Equal(t, 3, edge.SpanCount)
			require.Equal(t, 2, edge.TraceCount)
			require.Equal(t, 1, edge.ErrorSpanCount)
		}
	}
	require.NotEmpty(t, workerIdentity)
	children := read(map[string]any{"kind": "connection-spans", "serviceIdentity": identity, "targetIdentity": workerIdentity})
	require.Len(t, children.Rows, 3)
	require.Contains(t, string(children.Rows[0][0]), `"spanID":"0200000000000000"`)
	evidence := map[string]any{"kind": "connection-spans", "serviceIdentity": identity, "targetIdentity": workerIdentity, "limit": 1, "offset": 1}
	require.Equal(t, children.Rows[1:2], read(evidence).Rows)
	require.True(t, read(evidence).Truncated)
	evidence["offset"] = 2
	require.False(t, read(evidence).Truncated)
	options["queries"] = map[string]any{"traces": map[string]any{"type": "condition", "query": map[string]any{"field": map[string]any{"searchScope": "global"}, "fieldOperator": "CONTAINS", "value": "worker"}}}
	require.Empty(t, read(options).Rows) // child matches, but its parent does not
	delete(options, "queries")
	options["startTime"] = "110" // excludes the parent: no inferred cross-window edge
	page = read(options)
	require.Len(t, page.Rows, 1)
	options["queries"] = map[string]any{"traces": false}
	require.Empty(t, read(options).Rows)
	_, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connections"}}))
	require.Error(t, err)
	_, err = h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connection-spans", "serviceIdentity": identity}}))
	require.Error(t, err)
}
