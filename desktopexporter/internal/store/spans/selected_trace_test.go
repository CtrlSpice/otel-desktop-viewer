package spans_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSelectedTraceView(t *testing.T) {
	s, ctx := storetest.New(t)
	traceID := "000000000000000000000000000000a1"
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "agent")
	ss := rs.ScopeSpans().AppendEmpty()
	for index, parent := range []uint64{0, 1, 2, 1, 3, 99, 8, 7} {
		id := uint64(index + 1)
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(mustDecodeTraceID(traceID))
		sp.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", id)))
		if parent != 0 {
			sp.SetParentSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", parent)))
		}
		sp.SetName(fmt.Sprintf("span-%d", id))
		sp.SetStartTimestamp(pcommon.Timestamp(1700000000000000001 + id))
		sp.SetEndTimestamp(pcommon.Timestamp(1700000000000000101 + id))
		sp.Attributes().PutStr("payload", fmt.Sprintf("payload-%d", id))
		sp.Events().AppendEmpty().SetName(fmt.Sprintf("event-%d", id))
	}
	_, err := ingestReport(t, s, tr)
	require.NoError(t, err)
	_, err = ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000b2", "other-trace"))
	require.NoError(t, err)
	for _, selection := range [][]uint64{{1, 3, 5, 6, 7, 8, 123}, {3, 5}, {123}} {
		t.Run(fmt.Sprint(selection), func(t *testing.T) {
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return spans.GetTraceView(ctx, db, traceID, nil, selection...)
			})
			require.NoError(t, err)
			var got struct {
				TraceStart string `json:"traceStart"`
				Unplaced   int    `json:"unplacedSpanCount"`
				Spans      []struct {
					Depth int `json:"depth"`
					Span  struct {
						Name     string `json:"name"`
						Parent   string `json:"parentSpanID"`
						Duration string `json:"dur"`
					} `json:"spanData"`
				} `json:"spans"`
			}
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Zero(t, got.Unplaced)
			require.NotContains(t, string(raw), "other-trace")
			require.NotContains(t, string(raw), "payload-2")
			require.NotContains(t, string(raw), "event-4")
			if len(selection) == 1 {
				require.Empty(t, got.Spans)
				require.Equal(t, "0", got.TraceStart)
				return
			}
			if len(selection) == 2 {
				require.Len(t, got.Spans, 2)
				require.Equal(t, []int{0, 1}, []int{got.Spans[0].Depth, got.Spans[1].Depth})
				require.Equal(t, "1700000000000000004", got.TraceStart)
			} else {
				require.Len(t, got.Spans, 6)
				require.Equal(t, []int{0, 1, 2, 0, 0, 1}, []int{got.Spans[0].Depth, got.Spans[1].Depth, got.Spans[2].Depth, got.Spans[3].Depth, got.Spans[4].Depth, got.Spans[5].Depth})
			}
			for _, node := range got.Spans {
				require.Equal(t, "100", node.Span.Duration)
				if node.Span.Name == "span-3" {
					require.Equal(t, "0000000000000002", node.Span.Parent)
				}
			}
		})
	}
}
