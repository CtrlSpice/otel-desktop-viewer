package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanMembershipExecutionAndExistentialSemantics(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, createTestTracePdata())
	}))
	const traceID = "00000000000000000000000000000099"

	getTraceView := func(name, operator, value string) json.RawMessage {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: name, SearchScope: "field"},
			FieldOperator: operator,
			Value:         value,
		}}
		var raw json.RawMessage
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = spans.GetTraceView(ctx, db, traceID, query)
			return err
		}))
		return raw
	}
	matchedNames := func(raw json.RawMessage) []string {
		t.Helper()
		var trace struct {
			Spans []struct {
				Matched  bool `json:"matched"`
				SpanData struct {
					Name string `json:"name"`
				} `json:"spanData"`
			} `json:"spans"`
		}
		require.NoError(t, json.Unmarshal(raw, &trace))
		var names []string
		for _, span := range trace.Spans {
			if span.Matched {
				names = append(names, span.SpanData.Name)
			}
		}
		return names
	}

	t.Run("duration list", func(t *testing.T) {
		raw := getTraceView("duration", "IN", `["1s"]`)
		assert.Equal(t, []string{"root-operation"}, matchedNames(raw))
	})

	t.Run("nullable NOT IN excludes SQL null", func(t *testing.T) {
		raw := getTraceView("parentSpanID", "NOT IN", `["not-a-span"]`)
		assert.Len(t, matchedNames(raw), 8, "the root has a SQL-null parent and must not match")
	})

	t.Run("multi-event NOT IN stays existential", func(t *testing.T) {
		raw := getTraceView("event.name", "NOT IN", `["root-event-1"]`)
		assert.ElementsMatch(t, []string{"root-operation", "child-operation"}, matchedNames(raw), "another event on the same span satisfies NOT IN")
	})

	t.Run("wire ID list malformed value matches nothing", func(t *testing.T) {
		raw := getTraceView("spanID", "IN", `["not-a-span"]`)
		assert.Empty(t, matchedNames(raw))
	})

	t.Run("received numeric enum lists", func(t *testing.T) {
		assert.Equal(t, []string{"root-operation"}, matchedNames(getTraceView("kindCode", "IN", `["2"]`)))
		assert.Equal(t, []string{
			"child-operation",
			"great-grandchild-operation",
			"orphaned-grandchild-operation",
		}, matchedNames(getTraceView("statusCodeValue", "IN", `["2"]`)))
	})

	t.Run("all mapper modes execute with NOT IN", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			value string
		}{
			{"name", `["absent"]`},
			{"flags", `["99"]`},
			{"duration", `["99ns"]`},
			{"spanID", `["ffffffffffffffff"]`},
		} {
			assert.NotEmpty(t, getTraceView(tc.name, "NOT IN", tc.value), tc.name)
		}
	})
}

func TestNativeRootKeepsSpanIDsTraceLocal(t *testing.T) {
	s, ctx := storetest.New(t)
	data := ptrace.NewTraces()
	items := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for i, name := range []string{"alpha", "beta"} {
		span := items.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{255, 255, 255, 255, 255, 255, 255, 255})
		span.SetName(name)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
	for i, name := range []string{"alpha", "beta"} {
		sqlText, args, err := spans.MatchingRowsSQL(&search.QueryNode{Type: "text", Query: &search.Query{FieldOperator: "CONTAINS", Value: name}}, timerange.TimeRange{})
		require.NoError(t, err)
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.QueryContext(ctx, "select trace_id::varchar from ("+sqlText+")", args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			var found []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				found = append(found, strings.ReplaceAll(id, "-", ""))
			}
			require.Equal(t, []string{pcommon.TraceID{byte(i + 1)}.String()}, found)
			return rows.Err()
		}))
	}
}

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
		sp.SetTraceID(storetest.MustDecodeTraceID(traceID))
		sp.SetSpanID(storetest.MustDecodeSpanID(fmt.Sprintf("%016x", id)))
		if parent != 0 {
			sp.SetParentSpanID(storetest.MustDecodeSpanID(fmt.Sprintf("%016x", parent)))
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
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
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

// namedSpans builds one trace holding each name count-many times, so
// frequency ordering has something to order.
func namedSpans(names map[string]int) ptrace.Traces {
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "names")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("sc")
	traceID := storetest.MustDecodeTraceID("000000000000000000000000000000dd")
	i := 0
	for name, count := range names {
		for range count {
			i++
			sp := ss.Spans().AppendEmpty()
			sp.SetTraceID(traceID)
			sp.SetSpanID(storetest.MustDecodeSpanID(fmt.Sprintf("%016x", i)))
			sp.SetName(name)
			sp.SetStartTimestamp(pcommon.Timestamp(1000))
			sp.SetEndTimestamp(pcommon.Timestamp(2000))
		}
	}
	return tr
}

func spanNames(t *testing.T, s *store.Store, term string, limit int64) []string {
	t.Helper()
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetFieldValueCompletions(t.Context(), db, "name", term, limit)
	})
	require.NoError(t, err)
	var out []string
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestGetSpanNames(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, namedSpans(map[string]int{
			"checkout/pay":    5,
			"checkout/verify": 3,
			"db/select 100%":  2,
			"auth/login":      1,
		}))
	}))

	// Most frequent first; ties broken by name.
	assert.Equal(t,
		[]string{"checkout/pay", "checkout/verify", "db/select 100%", "auth/login"},
		spanNames(t, s, "", 10))

	// Substring match, case-insensitive.
	assert.Equal(t,
		[]string{"checkout/pay", "checkout/verify"},
		spanNames(t, s, "CHECK", 10))

	// A literal % in the term matches itself, not everything.
	assert.Equal(t, []string{"db/select 100%"}, spanNames(t, s, "100%", 10))
	assert.Empty(t, spanNames(t, s, "%zzz%", 10))

	// The limit is a limit.
	assert.Len(t, spanNames(t, s, "", 2), 2)

	// No match is an empty array, not null.
	assert.NotNil(t, spanNames(t, s, "no-such-name", 10))
}

// A field outside the allowlist is an error, not an empty list: the allowlist
// is the security boundary, and a typo should be loud.
func TestGetFieldValueCompletionsRefusesUnknownField(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	_, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetFieldValueCompletions(ctx, db, "statusMessage", "", 10)
	})
	require.ErrorIs(t, err, spans.ErrInvalidTraceQuery)
}

func TestAttributeDefinitionsStayWithinRequestedTrace(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := ptrace.NewTraces()
	traceIDs := []pcommon.TraceID{{1}, {2}}
	type definition struct {
		Name  string `json:"name"`
		Scope string `json:"attributeScope"`
		Type  string `json:"type"`
	}
	expected := make([][]definition, len(traceIDs))
	var all []definition
	for i, traceID := range traceIDs {
		label := []string{"first", "second"}[i]
		rm := data.ResourceSpans().AppendEmpty()
		sm := rm.ScopeSpans().AppendEmpty()
		span := sm.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(pcommon.SpanID{1})
		event := span.Events().AppendEmpty()
		link := span.Links().AppendEmpty()
		link.SetTraceID(traceIDs[1-i])
		link.SetSpanID(pcommon.SpanID{1})
		for _, owner := range []struct {
			scope string
			attrs pcommon.Map
		}{
			{"resource", rm.Resource().Attributes()},
			{"scope", sm.Scope().Attributes()},
			{"span", span.Attributes()},
			{"event", event.Attributes()},
			{"link", link.Attributes()},
		} {
			key := owner.scope + "." + label
			owner.attrs.PutStr(key, "value")
			owner.attrs.PutStr("shared", "same")
			only := definition{key, owner.scope, "string"}
			shared := definition{"shared", owner.scope, "string"}
			expected[i] = append(expected[i], only, shared)
			all = append(all, only)
			if i == 0 {
				all = append(all, shared)
			}
		}
	}
	// A retained trace without attributes must also return an empty list.
	emptyTraceID := pcommon.TraceID{3}
	empty := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	empty.SetTraceID(emptyTraceID)
	empty.SetSpanID(pcommon.SpanID{1})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for i, id := range traceIDs {
			raw, err := spans.GetTraceAttributeDefinitionsByTraceID(ctx, db, id.String())
			require.NoError(t, err)
			var got []definition
			require.NoError(t, json.Unmarshal(raw, &got))
			assert.ElementsMatch(t, expected[i], got)
		}
		for _, id := range []pcommon.TraceID{emptyTraceID, {4}} {
			raw, err := spans.GetTraceAttributeDefinitionsByTraceID(ctx, db, id.String())
			require.NoError(t, err)
			assert.JSONEq(t, "[]", string(raw))
		}
		raw, err := spans.GetTraceAttributeDefinitions(ctx, db)
		require.NoError(t, err)
		var got []definition
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.ElementsMatch(t, all, got)
		return nil
	}))
}

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
				span.SetTraceID(storetest.MustDecodeTraceID(traceID))
				span.SetSpanID(storetest.MustDecodeSpanID(spanID))
				span.SetName(name)
				span.SetStartTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000))
				span.SetEndTimestamp(pcommon.Timestamp(1_700_000_000_001_000_000))
				return span
			}
			addLink := func(owner ptrace.Span, traceID, spanID string) {
				link := owner.Links().AppendEmpty()
				if traceID != "" {
					link.SetTraceID(storetest.MustDecodeTraceID(traceID))
				}
				if spanID != "" {
					link.SetSpanID(storetest.MustDecodeSpanID(spanID))
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
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
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
			require.Equal(t, 6, storetest.CountRows(t, s, ctx, "select count(*) from span_link_summaries(?::uuid, ?::ubigint)", ownerTrace, 1))
			require.Equal(t, 7, storetest.CountRows(t, s, ctx, "select count(*) from span_link_summaries(?::uuid)", ownerTrace))
			export := func() json.RawMessage {
				t.Helper()
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
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
