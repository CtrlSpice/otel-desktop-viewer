package spans_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type linkSummarySpan struct {
	Name        string `json:"name"`
	ServiceName string `json:"serviceName"`
}

type projectedLink struct {
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
	Flags   uint32 `json:"flags"`
	Summary struct {
		Target *linkSummarySpan `json:"target"`
	} `json:"summary"`
}

func TestSpanLinkSummaries(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		name := "ordinary trace"
		if cycle {
			name = "salvaged trace"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, ctx := storetest.New(t)
			const ownerTrace = "000000000000000000000000000000a1"
			const targetTrace = "000000000000000000000000000000b2"
			const wrongTrace = "000000000000000000000000000000c3"
			const missingTrace = "000000000000000000000000000000d4"
			const ownerID = "0000000000000001"
			const siblingID = "0000000000000002"
			const targetID = "ffffffffffffffff"

			addSpan := func(data ptrace.Traces, traceID, spanID, name, service string) ptrace.Span {
				rs := data.ResourceSpans().AppendEmpty()
				if service != "" {
					rs.Resource().Attributes().PutStr("service.name", service)
				}
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.SetTraceID(mustDecodeTraceID(traceID))
				span.SetSpanID(mustDecodeSpanID(spanID))
				span.SetName(name)
				span.SetStartTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000))
				span.SetEndTimestamp(pcommon.Timestamp(1_700_000_000_001_000_000))
				return span
			}
			addLink := func(owner ptrace.Span, traceID, spanID string) {
				link := owner.Links().AppendEmpty()
				if traceID != "" {
					link.SetTraceID(mustDecodeTraceID(traceID))
				}
				if spanID != "" {
					link.SetSpanID(mustDecodeSpanID(spanID))
				}
				link.SetFlags(257)
				link.Attributes().PutStr("relationship", "follows_from")
			}
			data := ptrace.NewTraces()
			owner := addSpan(data, ownerTrace, ownerID, "publish", "checkout")
			if cycle {
				owner.SetParentSpanID(owner.SpanID())
			}
			sibling := addSpan(data, ownerTrace, siblingID, "local work", "")
			addSpan(data, wrongTrace, targetID, "wrong span", "wrong service")
			addLink(owner, targetTrace, targetID)
			addLink(owner, targetTrace, targetID) // Repeated links must not merge.
			addLink(owner, ownerTrace, siblingID)
			addLink(owner, missingTrace, targetID)
			addLink(owner, targetTrace, "")
			addLink(owner, "", targetID)
			addLink(sibling, missingTrace, targetID)
			_, err := ingestReport(t, s, data)
			require.NoError(t, err)

			readLinks := func(view bool) []projectedLink {
				t.Helper()
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					if view {
						return spans.GetTraceView(ctx, db, ownerTrace, nil)
					}
					return spans.GetSpan(ctx, db, ownerTrace, 1)
				})
				require.NoError(t, err)
				var span struct {
					Links []projectedLink `json:"links"`
				}
				if view {
					var trace struct {
						Spans []struct {
							SpanData struct {
								SpanID string          `json:"spanID"`
								Links  []projectedLink `json:"links"`
							} `json:"spanData"`
						} `json:"spans"`
					}
					require.NoError(t, json.Unmarshal(raw, &trace))
					for _, node := range trace.Spans {
						if node.SpanData.SpanID == ownerID {
							span.Links = node.SpanData.Links
						}
					}
				} else {
					require.NoError(t, json.Unmarshal(raw, &span))
				}
				require.Len(t, span.Links, 6)
				return span.Links
			}
			check := func(targetPresent bool) {
				t.Helper()
				for _, view := range []bool{true, false} {
					var found, local, missing int
					for _, link := range readLinks(view) {
						require.Equal(t, uint32(257), link.Flags)
						switch {
						case link.TraceID == targetTrace && link.SpanID == targetID:
							found++
							if targetPresent {
								require.Equal(t, &linkSummarySpan{"consume", "worker"}, link.Summary.Target)
							} else {
								require.Nil(t, link.Summary.Target)
							}
						case link.TraceID == ownerTrace && link.SpanID == siblingID:
							local++
							require.Equal(t, &linkSummarySpan{"local work", ""}, link.Summary.Target)
						default:
							missing++
							// A missing trace or either missing ID must not borrow another target.
							require.Nil(t, link.Summary.Target)
						}
					}
					require.Equal(t, 2, found)
					require.Equal(t, 1, local)
					require.Equal(t, 3, missing)
				}
			}
			check(false)
			require.Equal(t, 6, countRows(t, s, ctx, "select count(*) from span_link_summaries(?::uuid, ?::ubigint)", ownerTrace, 1))
			require.Equal(t, 7, countRows(t, s, ctx, "select count(*) from span_link_summaries(?::uuid)", ownerTrace))
			export := func() json.RawMessage {
				t.Helper()
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					return spans.GetTraceOTLP(ctx, db, ownerTrace)
				})
				require.NoError(t, err)
				require.NotContains(t, string(raw), "summary")
				return raw
			}
			before := export()
			late := ptrace.NewTraces()
			addSpan(late, targetTrace, targetID, "consume", "worker")
			_, err = ingestReport(t, s, late)
			require.NoError(t, err)
			check(true)
			require.JSONEq(t, string(before), string(export()))
			require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, "delete from spans where trace_id = ?::uuid", targetTrace)
				return err
			}))
			check(false)
			require.JSONEq(t, string(before), string(export()))
		})
	}
}
