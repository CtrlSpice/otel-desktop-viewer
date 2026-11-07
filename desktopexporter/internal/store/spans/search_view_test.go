package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSearchSummaryAndViewAgree(t *testing.T) {
	s, ctx := storetest.New(t)
	data := ptrace.NewTraces()
	ss := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for i := range 2 {
		sp := ss.AppendEmpty()
		sp.SetTraceID(pcommon.TraceID{1})
		sp.SetSpanID(pcommon.SpanID{byte(i + 1)})
		sp.SetName("needle")
		sp.Events().AppendEmpty().Attributes().PutStr("a", "one")
		if i == 0 {
			sp.Events().AppendEmpty().Attributes().PutStr("b", "two")
		} else {
			sp.Events().At(0).Attributes().PutStr("b", "two")
		}
	}
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(ctx, c, data) }))
	text := search.QueryNode{Type: "text", Query: &search.Query{FieldOperator: "CONTAINS", Value: "needle"}}
	child := func(key, value string) search.QueryNode {
		return search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Owner: "event", Path: []any{"attributes", key}, Type: "string"}, FieldOperator: "=", Value: value}}
	}
	for _, tc := range []struct {
		name    string
		query   search.QueryNode
		matched []string
	}{
		{"text", text, []string{ss.At(0).SpanID().String(), ss.At(1).SpanID().String()}},
		{"one child", search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: []search.QueryNode{text, child("a", "one"), child("b", "two")}}}, []string{ss.At(1).SpanID().String()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				raw, err := spans.SearchTraceSummaries(ctx, db, timerange.TimeRange{}, &tc.query)
				require.NoError(t, err)
				var summaries []struct{ MatchedSpans []struct{ SpanID string } }
				require.NoError(t, json.Unmarshal(raw, &summaries))
				require.Len(t, summaries, 1)
				var summaryIDs []string
				for _, span := range summaries[0].MatchedSpans {
					summaryIDs = append(summaryIDs, span.SpanID)
				}
				require.ElementsMatch(t, tc.matched, summaryIDs)
				raw, err = spans.GetTraceView(ctx, db, ss.At(0).TraceID().String(), &tc.query)
				require.NoError(t, err)
				var view struct {
					Spans []struct {
						Matched  bool
						SpanData struct{ SpanID string }
					}
				}
				require.NoError(t, json.Unmarshal(raw, &view))
				require.Len(t, view.Spans, 2)
				var viewIDs []string
				for _, span := range view.Spans {
					if span.Matched {
						viewIDs = append(viewIDs, span.SpanData.SpanID)
					}
				}
				require.ElementsMatch(t, tc.matched, viewIDs)
				result, err := spans.WaterfallArrow(ctx, db, ss.At(0).TraceID().String(), &tc.query)
				require.NoError(t, err)
				result.Close()
				return nil
			}))
		})
	}
}
