package spans_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// readStore runs a query under the store's read lock and returns its result.
// Store exposes no raw *sql.DB: every read is ordered against ingest,
// retention, and Close.
func readStore[T any](s *store.Store, fn func(db *sql.DB) (T, error)) (T, error) {
	var out T
	err := s.WithDBRead(func(db *sql.DB) error {
		var err error
		out, err = fn(db)
		return err
	})
	return out, err
}

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

func countRows(t *testing.T, s *store.Store, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, query, args...).Scan(&n)
	}))
	return n
}

// queryIDs collects a single uuid column as strings, ready to feed straight back
// as `in (...)` parameters.
//
// The dictionary made this necessary: an assertion about "the attributes this
// span referenced" has to capture those ids *before* the span row is deleted,
// because afterwards there is no array left to unnest. Select the column as
// ::varchar -- the driver hands a raw uuid back as bytes, and the round trip as
// text is what lets the ids go back out as ordinary query parameters.
func queryIDs(t *testing.T, s *store.Store, ctx context.Context, query string, args ...any) []any {
	t.Helper()
	var out []any
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	}))
	return out
}

// placeholders builds "?, ?, ?" for an n-parameter `in (...)` clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// mustDecodeTraceID decodes a 32-char hex string to 16 bytes (trace ID).
func mustDecodeTraceID(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic("invalid trace ID hex: " + s)
	}
	var out [16]byte
	copy(out[:], b)
	return out
}

// mustDecodeSpanID decodes a 16-char hex string to 8 bytes (span ID).
func mustDecodeSpanID(s string) [8]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		panic("invalid span ID hex: " + s)
	}
	var out [8]byte
	copy(out[:], b)
	return out
}

// buildTracesForSummaryOrdering builds three traces with different start times for ordering tests.
// Returns trace IDs as hex strings in order: trace1 (middle), trace2 (oldest), trace3 (newest).
func buildTracesForSummaryOrdering(baseTime int64) (ptrace.Traces, string, string, string) {
	traces := ptrace.NewTraces()
	trace1Hex := "00000000000000000000000000000001"
	trace2Hex := "00000000000000000000000000000002"
	trace3Hex := "00000000000000000000000000000003"
	span1Hex := "0000000000000001"
	span2Hex := "0000000000000002"
	span3Hex := "0000000000000003"

	addOneSpan := func(tr ptrace.Traces, traceIDHex, spanIDHex, parentSpanIDHex, name string, start, end int64, serviceName string) {
		rs := tr.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", serviceName)
		ss := rs.ScopeSpans().AppendEmpty()
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(mustDecodeTraceID(traceIDHex))
		s.SetSpanID(mustDecodeSpanID(spanIDHex))
		if parentSpanIDHex != "" {
			s.SetParentSpanID(mustDecodeSpanID(parentSpanIDHex))
		}
		s.SetName(name)
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.Timestamp(start))
		s.SetEndTimestamp(pcommon.Timestamp(end))
	}

	// Trace 1: middle time (t+1)
	addOneSpan(traces, trace1Hex, span1Hex, "", "root middle", baseTime+time.Second.Nanoseconds(), baseTime+2*time.Second.Nanoseconds(), "service1")
	// Trace 2: oldest (t+0), no root (parent missing)
	addOneSpan(traces, trace2Hex, span2Hex, "ffffffffffffffff", "earliest no root", baseTime, baseTime+2*time.Second.Nanoseconds(), "")
	// Trace 3: newest (t+2)
	addOneSpan(traces, trace3Hex, span3Hex, "", "root last", baseTime+2*time.Second.Nanoseconds(), baseTime+3*time.Second.Nanoseconds(), "service3")

	return traces,
		"00000000000000000000000000000001",
		"00000000000000000000000000000002",
		"00000000000000000000000000000003"
}

// searchTracesAll returns SearchTraces with a wide time range and nil query to get "all" summaries.
func searchTracesAll(t *testing.T, s *store.Store, ctx context.Context) []traceSummaryJSON {
	t.Helper()
	const maxNano = 1<<63 - 1
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraces(ctx, db, 0, maxNano, nil)
	})
	assert.NoError(t, err)
	var summaries []traceSummaryJSON
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	return summaries
}

type traceSummaryJSON struct {
	TraceID     string        `json:"traceID"`
	HasRootSpan bool          `json:"hasRootSpan"`
	RootSpan    *rootSpanJSON `json:"rootSpan"`
	StartTime   string        `json:"startTime"`  // varchar-encoded int64 ns
	DurationNs  *string       `json:"durationNs"` // string-encoded int64 ns; max(end) - min(start) over trace
	SpanCount   float64       `json:"spanCount"`  // JSON number
	ErrorCount  float64       `json:"errorCount"`
}

type rootSpanJSON struct {
	ServiceName string `json:"serviceName"`
	Name        string `json:"name"`
}

// TestTraceSummaryOrdering verifies that trace summaries are ordered by start time (newest first).
func TestTraceSummaryOrdering(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	traces, trace1Hex, trace2Hex, trace3Hex := buildTracesForSummaryOrdering(baseTime)

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err, "failed to ingest spans")

	summaries := searchTracesAll(t, s, ctx)
	assert.Len(t, summaries, 3, "expected 3 traces")

	// Order: trace3 (newest) -> trace1 -> trace2 (oldest)
	assert.Equal(t, trace3Hex, summaries[0].TraceID, "first trace should be trace3 (latest start)")
	assert.Equal(t, trace1Hex, summaries[1].TraceID, "second trace should be trace1")
	assert.Equal(t, trace2Hex, summaries[2].TraceID, "last trace should be trace2 (earliest start)")

	assert.Nil(t, summaries[2].RootSpan, "trace2 should not have root span")
	assert.NotNil(t, summaries[1].RootSpan, "trace1 should have root span")
	assert.NotNil(t, summaries[0].RootSpan, "trace3 should have root span")

	// Orphan trace2: trace bounds from its only span (no root yet).
	assert.False(t, summaries[2].HasRootSpan)
	assert.NotNil(t, summaries[2].DurationNs, "orphan trace should have span-bounds duration")
	const twoSecondsNs = "2000000000"
	assert.Equal(t, twoSecondsNs, *summaries[2].DurationNs)
	assert.Equal(t, fmt.Sprintf("%d", baseTime), summaries[2].StartTime)
}

// TestTraceNotFound verifies error handling for non-existent trace IDs.
func TestTraceNotFound(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "00000000-0000-0000-0000-000000000000", nil)
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, spans.ErrTraceIDNotFound)
}

// TestEmptySpans verifies handling of empty span lists and empty stores.
func TestEmptySpans(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, ptrace.NewTraces())
	})
	assert.NoError(t, err)

	summaries := searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
}

// TestClearTraces verifies that all traces can be cleared from the store,
// including child rows, and pins the two-step contract the dictionary
// introduced: Clear drops the owners, the sweep collects what that orphaned.
//
// Clear used to delete the attribute rows it owned, because every attribute row
// belonged to exactly one span/event/link and "is it still needed?" had an
// obvious answer. Attributes are now shared across spans, logs and metrics, so
// Clear cannot answer that question and deliberately leaves them behind --
// asserting they *survive* is the point, not an omission. ingest.SweepOrphans
// is the only thing that may delete them.
func TestClearTraces(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	summaries := searchTracesAll(t, s, ctx)
	assert.Len(t, summaries, 1)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from events"), 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from links"), 0)

	// Snapshot the dictionary so the post-Clear assertion is "unchanged", not
	// merely "non-empty" -- a Clear that deleted some but not all attribute rows
	// would slip past a non-empty check.
	attrsBefore := countRows(t, s, ctx, "select count(*) from attributes")
	assert.Greater(t, attrsBefore, 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from attributes where scope = 'span'"), 0)

	err = s.WithDBWrite(func(db *sql.DB) error {
		return spans.Clear(ctx, db)
	})
	assert.NoError(t, err)

	summaries = searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from links"))

	// The dictionary, resources and scopes are untouched by Clear.
	assert.Equal(t, attrsBefore, countRows(t, s, ctx, "select count(*) from attributes"),
		"Clear must not delete attribute rows: they are shared with logs and metrics")
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from resources"), 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from scopes"), 0)

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db)
	}))

	// Spans were the only signal ingested, so after the sweep nothing is
	// referenced and the three tables empty out completely.
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from attributes"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from resources"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from scopes"))
}

// getTraceTraceID returns the trace ID from SearchSpans JSON (traceID in response is hex string).
func getTraceTraceID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var out struct {
		TraceID string `json:"traceID"`
	}
	assert.NoError(t, json.Unmarshal(raw, &out))
	return out.TraceID
}

// getTraceSpansCount returns the number of spans in SearchSpans JSON.
func getTraceSpansCount(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var out struct {
		Spans []json.RawMessage `json:"spans"`
	}
	assert.NoError(t, json.Unmarshal(raw, &out))
	return len(out.Spans)
}

// spanDataFromSearchSpans returns spanData.name and spanID for the i-th span (depth-first order).
func spanDataFromSearchSpans(t *testing.T, raw json.RawMessage, i int) (name, spanID string) {
	t.Helper()
	var out struct {
		Spans []struct {
			SpanData struct {
				Name   string `json:"name"`
				SpanID string `json:"spanID"`
			} `json:"spanData"`
		} `json:"spans"`
	}
	assert.NoError(t, json.Unmarshal(raw, &out))
	assert.GreaterOrEqual(t, len(out.Spans), i+1)
	return out.Spans[i].SpanData.Name, out.Spans[i].SpanData.SpanID
}

// TestTraceSuite runs a comprehensive suite of tests on a single trace.
func TestTraceSuite(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	testTraceID := "00000000000000000000000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err, "failed to ingest test trace")

	t.Run("TraceHierarchicalStructure", func(t *testing.T) {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchSpans(ctx, db, testTraceID, nil)
		})
		assert.NoError(t, err, "failed to get trace")
		assert.NotEmpty(t, raw)

		assert.Equal(t, testTraceID, getTraceTraceID(t, raw))
		assert.Equal(t, 9, getTraceSpansCount(t, raw), "should have 9 spans")

		// Depth-first order: root -> child -> grandchild -> great-grandchild -> child-span-2 -> child2-child -> orphaned -> orphaned-child -> orphaned-grandchild
		names := []string{"root-operation", "child-operation", "grandchild-operation", "great-grandchild-operation", "child-operation-2", "child2-child-operation", "orphaned-operation", "orphaned-child-operation", "orphaned-grandchild-operation"}
		for i, want := range names {
			name, _ := spanDataFromSearchSpans(t, raw, i)
			assert.Equal(t, want, name, "span index %d", i)
		}
	})

	t.Run("TraceSummary", func(t *testing.T) {
		summaries := searchTracesAll(t, s, ctx)
		assert.Len(t, summaries, 1, "should have one trace summary")

		summary := summaries[0]
		assert.Equal(t, testTraceID, summary.TraceID)
		assert.Equal(t, float64(9), summary.SpanCount)
		assert.NotNil(t, summary.RootSpan)
		assert.Equal(t, "test-service", summary.RootSpan.ServiceName)
		assert.Equal(t, "root-operation", summary.RootSpan.Name)
	})

	t.Run("TraceNotFound", func(t *testing.T) {
		_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchSpans(ctx, db, "00000000-0000-0000-0000-000000000000", nil)
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, spans.ErrTraceIDNotFound)
	})

	t.Run("SearchSpansAcceptsTraceIDWithoutHyphens", func(t *testing.T) {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchSpans(ctx, db, "00000000000000000000000000000099", nil)
		})
		assert.NoError(t, err, "SearchSpans with 32-char hex trace ID should succeed")
		assert.Equal(t, testTraceID, getTraceTraceID(t, raw))
	})

	t.Run("AttributeDiscovery", func(t *testing.T) {
		now := time.Now().UnixNano()
		start := now - 24*int64(time.Hour)
		end := now + 24*int64(time.Hour)
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.GetTraceAttributes(ctx, db, start, end)
		})
		assert.NoError(t, err, "failed to get trace attributes")

		var attributes []struct {
			Name           string `json:"name"`
			AttributeScope string `json:"attributeScope"`
			Type           string `json:"type"`
		}
		assert.NoError(t, json.Unmarshal(raw, &attributes))
		assert.NotEmpty(t, attributes, "should have discovered attributes")

		byScope := make(map[string][]string)
		byScopeType := make(map[string]string)
		for _, a := range attributes {
			byScope[a.AttributeScope] = append(byScope[a.AttributeScope], a.Name)
			byScopeType[a.Name] = a.Type
		}

		for _, scope := range []string{"resource", "span", "event", "link"} {
			assert.Contains(t, byScope, scope, "should have %s attributes", scope)
		}
		assert.Contains(t, byScope["resource"], "service.name")
		assert.Contains(t, byScope["resource"], "service.version")
		assert.Contains(t, byScope["span"], "root.string")
		assert.Contains(t, byScope["span"], "root.int")
		assert.Contains(t, byScope["span"], "root.float")
		assert.Contains(t, byScope["span"], "root.bool")
		assert.Contains(t, byScope["span"], "root.list")
		assert.Contains(t, byScope["event"], "event.string")
		assert.Contains(t, byScope["event"], "event.int")
		assert.Contains(t, byScope["link"], "link.string")
		assert.Contains(t, byScope["link"], "link.int")

		assert.Equal(t, "string", byScopeType["service.name"])
		assert.Equal(t, "int64", byScopeType["root.int"])
		assert.Equal(t, "float64", byScopeType["root.float"])
		assert.Equal(t, "bool", byScopeType["root.bool"])
		assert.Equal(t, "string[]", byScopeType["root.list"])
	})
}

// TestSearchTraces tests SearchTraces with various query types.
func TestSearchTraces(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	testTraceID := "00000000000000000000000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err, "failed to ingest test trace")

	baseTime := time.Now().UnixNano()
	startTime := baseTime - 24*int64(time.Hour)
	endTime := baseTime + 24*int64(time.Hour)

	parseSummaries := func(raw json.RawMessage) []traceSummaryJSON {
		var s []traceSummaryJSON
		assert.NoError(t, json.Unmarshal(raw, &s))
		return s
	}

	t.Run("GlobalSearch_ResourceAttribute", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q1",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "test-service",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_SpanAttribute", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q2",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "root-value",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_EventField", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "root-event",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_EventAttribute", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q4",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "Hello",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_LinkAttribute", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "Link1",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_SpanID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q7",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "0000000000000001",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries, "global search for span ID hex should match")
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_TraceID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q8",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "00000000000000000000000000000099",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries, "global search for trace ID hex should match")
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q6",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "nonexistent-value-12345",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.Empty(t, summaries)
	})

	t.Run("ResourceAttribute_ServiceName", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q9",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "service.name",
					SearchScope:    "attribute",
					AttributeScope: "resource",
					Type:           "string",
				},
				FieldOperator: "CONTAINS",
				Value:         "test-service",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_Int64", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q10",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.int",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "int64",
				},
				FieldOperator: "=",
				Value:         "42",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_Float64", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q11",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.float",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "float64",
				},
				FieldOperator: "=",
				Value:         "3.14",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_Boolean", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q12",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.bool",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "boolean",
				},
				FieldOperator: "=",
				Value:         "true",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_StringArray", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q13",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.list",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "string[]",
				},
				FieldOperator: "CONTAINS",
				Value:         "two",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_Int64Array", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q13b",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.int_list",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "int64[]",
				},
				FieldOperator: "CONTAINS",
				Value:         "20",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_Float64Array", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q13c",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.float_list",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "float64[]",
				},
				FieldOperator: "CONTAINS",
				Value:         "2.2",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("SpanAttribute_BooleanArray", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q13d",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "root.bool_list",
					SearchScope:    "attribute",
					AttributeScope: "span",
					Type:           "boolean[]",
				},
				FieldOperator: "CONTAINS",
				Value:         "true",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("EventAttribute_String", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q15",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "event.string",
					SearchScope:    "attribute",
					AttributeScope: "event",
					Type:           "string",
				},
				FieldOperator: "=",
				Value:         "Hello",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("LinkAttribute_String", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q16",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "link.string",
					SearchScope:    "attribute",
					AttributeScope: "link",
					Type:           "string",
				},
				FieldOperator: "=",
				Value:         "Link1",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	// QueryByServiceName: exercise ParseQueryTree(query) with map input and BuildTraceSQL (resource attribute).
	t.Run("QueryByServiceName", func(t *testing.T) {
		query := map[string]any{
			"id":   "qs1",
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name":           "service.name",
					"searchScope":    "attribute",
					"attributeScope": "resource",
				},
				"fieldOperator": "=",
				"value":         "test-service",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	// Field expression tests (mapTraceFieldExpression cases)
	t.Run("Field_Name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f1",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "root-operation",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.Len(t, summaries, 1)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
		assert.NotNil(t, summaries[0].RootSpan)
		assert.Equal(t, "root-operation", summaries[0].RootSpan.Name)
	})

	t.Run("Field_TraceID", func(t *testing.T) {
		query := map[string]any{
			"id":   "f2",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         testTraceID,
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.Len(t, summaries, 1)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_scope.name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f3",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scope.name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_scope.version", func(t *testing.T) {
		query := map[string]any{
			"id":   "f4",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scope.version", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_event.name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f5",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "event.name", "searchScope": "field"},
				"fieldOperator": "CONTAINS",
				"value":         "root-event",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_link.traceID", func(t *testing.T) {
		// Link from root span: linkedTraceID = 0000000000000000000000000000000a -> UUID 00000000-0000-0000-0000-00000000000a
		query := map[string]any{
			"id":   "f6",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "link.traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "00000000-0000-0000-0000-00000000000a",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_link.traceID_WireForm", func(t *testing.T) {
		// Same link, queried with the dash-less 32-hex form the API serves.
		query := map[string]any{
			"id":   "f7",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "link.traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "0000000000000000000000000000000a",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	t.Run("Field_link.spanID_WireForm", func(t *testing.T) {
		// link.spanID means the linked *target* (matching the spanID field
		// in served link JSON), queried in 16-hex wire form. Root span's
		// link targets 000000000000000a. Before the linked_span_id alias +
		// wire-form conversion this hit the owner column and errored on
		// the uuid cast.
		query := map[string]any{
			"id":   "f8",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "link.spanID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "000000000000000a",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
	})

	// Malformed IDs are a search that finds nothing, not a uuid cast error
	// bubbling up as -32603 (issue #276).
	t.Run("Field_traceID_Garbage", func(t *testing.T) {
		query := map[string]any{
			"id":   "f9",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "not-a-trace",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err, "garbage trace ID must not surface a cast error")
		assert.Empty(t, parseSummaries(raw))
	})

	t.Run("Field_spanID_Garbage", func(t *testing.T) {
		query := map[string]any{
			"id":   "f10",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "spanID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "zz-definitely-not-hex",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err, "garbage span ID must not surface a cast error")
		assert.Empty(t, parseSummaries(raw))
	})

	t.Run("Field_link.traceID_Garbage", func(t *testing.T) {
		query := map[string]any{
			"id":   "f11",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "link.traceID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "not-a-trace",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err, "garbage link trace ID must not surface a cast error")
		assert.Empty(t, parseSummaries(raw))
	})
}

// TestIngestSpans_FlushInterval exercises the flushIntervalSpans codepath by ingesting
// more than 50 spans in one call (flush runs when spanCount % 50 == 0). All spans have
// resource, scope, and span attributes; we assert they were flushed correctly.
func TestIngestSpans_FlushInterval(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	const batchSize = 51 // > flushIntervalSpans (50)
	traces := createTestTracesPdataN(batchSize)
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	testTraceID := "00000000000000000000000000000099"
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, testTraceID, nil)
	})
	assert.NoError(t, err)
	assert.Equal(t, batchSize, getTraceSpansCount(t, raw))

	// Assert attributes flushed: span 1 (index 0), span 50 (index 49), span 51 (index 50)
	// SpanID for index i is (i+1) as 16-char hex; UUID format is 8-4-4-4-12.
	//
	// Attributes are no longer rows owned by a span, so "did this span's
	// attributes survive the flush?" is now asked by unnesting the span's
	// attribute_ids and joining the dictionary. The join matters: it fails if the
	// appender wrote an array whose ids never made it into the dictionary, which
	// is precisely the split-brain the two-pass ingest could produce if a
	// mid-batch flush landed between the passes.
	for _, spanIndex := range []int{0, 49, 50} {
		spanIDHex := fmt.Sprintf("%016x", spanIndex+1)
		spanUUID := "00000000-0000-0000-0000-" + spanIDHex[4:]
		attrCount := countRows(t, s, ctx, `
			select count(*)
			from (select unnest(attribute_ids) as id from spans where span_id = ?) x
			join attributes a on a.id = x.id
			where a.scope = 'span' and a.key in ('span.index', 'flush_test')
		`, spanUUID)
		assert.GreaterOrEqual(t, attrCount, 2, "span %d should have span.index and flush_test attributes", spanIndex)
	}

	// Resource/scope attributes reached through the first span's resource_id and
	// scope_id. These used to be re-written once per owning span; they are now
	// one deduped row each, so the assertion goes through the reference rather
	// than looking for a span-keyed copy.
	span1UUID := "00000000-0000-0000-0000-000000000001"
	resAttr := countRows(t, s, ctx, `
		select count(*)
		from spans s
		join resources r on r.id = s.resource_id
		join (select id, key, scope from attributes) a
		  on list_contains(r.attribute_ids, a.id)
		where s.span_id = ? and a.scope = 'resource'
	`, span1UUID)
	scopeAttr := countRows(t, s, ctx, `
		select count(*)
		from spans s
		join scopes sc on sc.id = s.scope_id
		join (select id, key, scope from attributes) a
		  on list_contains(sc.attribute_ids, a.id)
		where s.span_id = ? and a.scope = 'scope'
	`, span1UUID)
	assert.GreaterOrEqual(t, resAttr, 1)
	assert.GreaterOrEqual(t, scopeAttr, 1)
}

func TestIngest_CanceledContext(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, createTestTracesPdataN(1))
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestIngest_CanceledDuringIngest(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())
	traces := createTestTracesPdataN(100)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.WithConn(func(conn driver.Conn) error {
			return spans.Ingest(ctx, conn, traces)
		})
	}()
	cancel()

	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
}

// TestDeleteSpansByIDs verifies that multiple spans can be deleted by their SpanIDs, including child rows.
func TestDeleteSpansByIDs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "00000000-0000-0000-0000-000000000099", nil)
	})
	assert.NoError(t, err)
	assert.Equal(t, 9, getTraceSpansCount(t, raw))

	deletedIDs := []any{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000003",
	}

	// Capture the dictionary ids these spans reference before they go, because
	// afterwards there is no span row left to unnest. Each of these three spans
	// carries its own distinctly-named attributes in the fixture (root.*,
	// child.*, child2.*), so every span-scoped id here is referenced by nothing
	// else -- which is what makes the post-sweep assertion below exact rather
	// than approximate.
	spanAttrIDs := queryIDs(t, s, ctx, `
		select distinct a.id::varchar
		from (select unnest(attribute_ids) as id from spans where span_id in (?, ?, ?)) x
		join attributes a on a.id = x.id
		where a.scope = 'span'
	`, deletedIDs...)
	require.NotEmpty(t, spanAttrIDs, "deleted spans should have attributes")
	inSpanAttrIDs := "select count(*) from attributes where id in (" + placeholders(len(spanAttrIDs)) + ")"

	attrsTotalBefore := countRows(t, s, ctx, "select count(*) from attributes")

	err = s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByIDs(ctx, db, deletedIDs)
	})
	assert.NoError(t, err)

	raw, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "00000000-0000-0000-0000-000000000099", nil)
	})
	assert.NoError(t, err)
	assert.Equal(t, 6, getTraceSpansCount(t, raw))

	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from events where span_id in (?, ?, ?)", deletedIDs...))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from links where span_id in (?, ?, ?)", deletedIDs...))

	// The delete-by-id path no longer removes attribute rows. It cannot: the
	// dictionary is shared, so deciding a row is dead needs a whole-database
	// view that a targeted delete does not have.
	assert.Equal(t, attrsTotalBefore, countRows(t, s, ctx, "select count(*) from attributes"),
		"DeleteSpansByIDs must leave the dictionary alone")
	assert.Equal(t, len(spanAttrIDs), countRows(t, s, ctx, inSpanAttrIDs, spanAttrIDs...))

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db)
	}))

	// Now the sweep collects them -- and only them.
	assert.Equal(t, 0, countRows(t, s, ctx, inSpanAttrIDs, spanAttrIDs...),
		"SweepOrphans must collect the attributes the deleted spans were the last referrers of")
	// The resource attributes survive, because the six remaining spans still
	// point at the same resource row. A sweep that over-collected would take
	// these with it, and service filtering would break silently.
	assert.Equal(t, 1, countRows(t, s, ctx,
		"select count(*) from attributes where scope = 'resource' and key = 'service.name'"),
		"shared resource attributes must survive: live spans still reference them")
	assert.Equal(t, 1, countRows(t, s, ctx, "select count(*) from resources"))
}

// TestDeleteSpansByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteSpansByIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByIDs(ctx, db, []any{})
	})
	assert.NoError(t, err)
}

// TestSearchSpansWith32CharHexTraceID verifies that SearchSpans finds a trace when given the 32-char hex form (no hyphens).
func TestSearchSpansWith32CharHexTraceID(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	require.NoError(t, err)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "00000000000000000000000000000099", nil)
	})
	assert.NoError(t, err, "SearchSpans with 32-char hex trace ID should succeed")
	assert.NotEmpty(t, raw)
	assert.Equal(t, "00000000000000000000000000000099", getTraceTraceID(t, raw))
}

// TestDeleteSpansByTraceIDs verifies that spans for multiple traces are deleted, including child rows.
func TestDeleteSpansByTraceIDs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	testTraceID := "00000000000000000000000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	summaries := searchTracesAll(t, s, ctx)
	assert.Len(t, summaries, 1)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from events"), 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from links"), 0)

	attrsBefore := countRows(t, s, ctx, "select count(*) from attributes")
	assert.Greater(t, attrsBefore, 0)

	err = s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(ctx, db, []any{testTraceID})
	})
	assert.NoError(t, err)

	summaries = searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from links"))

	// Same two-step contract as TestClearTraces: deleting the owners strands the
	// dictionary rows, and only the sweep may reclaim them. See the comment
	// there for why the delete path cannot make that call itself.
	assert.Equal(t, attrsBefore, countRows(t, s, ctx, "select count(*) from attributes"),
		"DeleteSpansByTraceIDs must leave the dictionary alone")

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db)
	}))

	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from attributes"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from resources"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from scopes"))
}

// TestDeleteSpansByTraceIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteSpansByTraceIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(ctx, db, []any{})
	})
	assert.NoError(t, err)
}

// createTestTracesPdataN builds one trace with n spans (one resource/scope). Each span has
// resource, scope, and span attributes. Used to exercise flushIntervalSpans by ingesting >= 50 spans.
func createTestTracesPdataN(n int) ptrace.Traces {
	baseTime := time.Now().UnixNano()
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	rs.Resource().Attributes().PutStr("resource.key", "resource.val")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("test-scope")
	ss.Scope().SetVersion("v1.0.0")
	ss.Scope().Attributes().PutStr("scope.key", "scope.val")
	traceID := mustDecodeTraceID("00000000000000000000000000000099")
	for i := 0; i < n; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(traceID)
		s.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", i+1)))
		s.SetParentSpanID([8]byte{})
		s.SetName("span-" + fmt.Sprintf("%d", i))
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.Timestamp(baseTime + int64(i)))
		s.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(i) + int64(time.Second)))
		s.Attributes().PutStr("span.index", fmt.Sprintf("%d", i))
		s.Attributes().PutStr("flush_test", "ok")
	}
	return tr
}

// createTestTracePdata builds the full 9-span test trace with events, links, and attributes (pdata).
func createTestTracePdata() ptrace.Traces {
	baseTime := time.Now().UnixNano()
	event1Time := baseTime + 100*int64(time.Millisecond)
	event2Time := baseTime + 200*int64(time.Millisecond)

	traceID := mustDecodeTraceID("00000000000000000000000000000099")
	rootSpanID := mustDecodeSpanID("0000000000000001")
	childSpanID := mustDecodeSpanID("0000000000000002")
	child2SpanID := mustDecodeSpanID("0000000000000003")
	grandchildSpanID := mustDecodeSpanID("0000000000000004")
	greatGrandchildSpanID := mustDecodeSpanID("0000000000000005")
	child2ChildSpanID := mustDecodeSpanID("0000000000000006")
	orphanedSpanID := mustDecodeSpanID("0000000000000007")
	orphanedChildSpanID := mustDecodeSpanID("0000000000000008")
	orphanedGrandchildSpanID := mustDecodeSpanID("0000000000000009")
	nonExistentParent := mustDecodeSpanID("ffffffffffffffff")
	linkedTraceID := mustDecodeTraceID("0000000000000000000000000000000a")
	linkedSpanID := mustDecodeSpanID("000000000000000a")
	linkedTraceID2 := mustDecodeTraceID("0000000000000000000000000000000b")
	linkedSpanID2 := mustDecodeSpanID("000000000000000b")

	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	rs.Resource().Attributes().PutStr("service.version", "1.0.0")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("test-scope")
	ss.Scope().SetVersion("v1.0.0")

	spans := ss.Spans()

	// Root span
	s0 := spans.AppendEmpty()
	s0.SetTraceID(traceID)
	s0.SetSpanID(rootSpanID)
	s0.SetParentSpanID([8]byte{})
	s0.SetName("root-operation")
	s0.SetKind(ptrace.SpanKindServer)
	s0.SetStartTimestamp(pcommon.Timestamp(baseTime))
	s0.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))
	s0.Attributes().PutStr("root.string", "root-value")
	s0.Attributes().PutInt("root.int", 42)
	s0.Attributes().PutDouble("root.float", 3.14)
	s0.Attributes().PutBool("root.bool", true)
	arr := s0.Attributes().PutEmptySlice("root.list")
	arr.AppendEmpty().SetStr("one")
	arr.AppendEmpty().SetStr("two")
	arr.AppendEmpty().SetStr("three")
	intArr := s0.Attributes().PutEmptySlice("root.int_list")
	intArr.AppendEmpty().SetInt(10)
	intArr.AppendEmpty().SetInt(20)
	intArr.AppendEmpty().SetInt(30)
	floatArr := s0.Attributes().PutEmptySlice("root.float_list")
	floatArr.AppendEmpty().SetDouble(1.1)
	floatArr.AppendEmpty().SetDouble(2.2)
	floatArr.AppendEmpty().SetDouble(3.3)
	boolArr := s0.Attributes().PutEmptySlice("root.bool_list")
	boolArr.AppendEmpty().SetBool(true)
	boolArr.AppendEmpty().SetBool(false)
	e0 := s0.Events().AppendEmpty()
	e0.SetName("root-event-1")
	e0.SetTimestamp(pcommon.Timestamp(event1Time))
	e0.Attributes().PutStr("event.string", "Hello")
	e0.Attributes().PutInt("event.int", 42)
	e0.Attributes().PutBool("event.bool", true)
	e0.Attributes().PutDouble("event.float", 3.14)
	e1 := s0.Events().AppendEmpty()
	e1.SetName("root-event-2")
	e1.SetTimestamp(pcommon.Timestamp(event2Time))
	e1.Attributes().PutStr("event.string2", "World")
	e1.Attributes().PutInt("event.int2", 100)
	arrE := e1.Attributes().PutEmptySlice("event.list")
	arrE.AppendEmpty().SetStr("a")
	arrE.AppendEmpty().SetStr("b")
	arrE.AppendEmpty().SetStr("c")
	l0 := s0.Links().AppendEmpty()
	l0.SetTraceID(linkedTraceID)
	l0.SetSpanID(linkedSpanID)
	l0.TraceState().FromRaw("state1")
	l0.Attributes().PutStr("link.string", "Link1")
	l0.Attributes().PutInt("link.int", 123)
	l0.Attributes().PutDouble("link.float", 2.71)
	l0.Attributes().PutBool("link.bool", false)
	s0.Status().SetCode(ptrace.StatusCodeOk)

	// Child span
	s1 := spans.AppendEmpty()
	s1.SetTraceID(traceID)
	s1.SetSpanID(childSpanID)
	s1.SetParentSpanID(rootSpanID)
	s1.SetName("child-operation")
	s1.SetKind(ptrace.SpanKindInternal)
	s1.SetStartTimestamp(pcommon.Timestamp(baseTime + 50*int64(time.Millisecond)))
	s1.SetEndTimestamp(pcommon.Timestamp(baseTime + 900*int64(time.Millisecond)))
	s1.Attributes().PutStr("child.string", "child-value")
	s1.Attributes().PutInt("child.int", 24)
	s1.Attributes().PutDouble("child.float", 2.71)
	s1.Attributes().PutBool("child.bool", false)
	arr1 := s1.Attributes().PutEmptySlice("child.list")
	arr1.AppendEmpty().SetInt(1)
	arr1.AppendEmpty().SetInt(2)
	arr1.AppendEmpty().SetInt(3)
	arr1.AppendEmpty().SetInt(4)
	arr1.AppendEmpty().SetInt(5)
	ex := s1.Events().AppendEmpty()
	ex.SetName("child-event")
	ex.SetTimestamp(pcommon.Timestamp(baseTime + 150*int64(time.Millisecond)))
	ex.Attributes().PutStr("child.event.string", "Child Event")
	ex.Attributes().PutInt("child.event.int", 50)
	ex.Attributes().PutBool("child.event.bool", false)
	ex.Attributes().PutDouble("child.event.float", 1.618)
	lx := s1.Links().AppendEmpty()
	lx.SetTraceID(linkedTraceID2)
	lx.SetSpanID(linkedSpanID2)
	lx.TraceState().FromRaw("state2")
	lx.Attributes().PutStr("child.link.string", "Child Link")
	lx.Attributes().PutInt("child.link.int", 456)
	lx.Attributes().PutDouble("child.link.float", 1.414)
	lx.Attributes().PutBool("child.link.bool", true)
	s1.Status().SetCode(ptrace.StatusCodeError)
	s1.Status().SetMessage("operation failed")

	// Child span 2
	s2 := spans.AppendEmpty()
	s2.SetTraceID(traceID)
	s2.SetSpanID(child2SpanID)
	s2.SetParentSpanID(rootSpanID)
	s2.SetName("child-operation-2")
	s2.SetKind(ptrace.SpanKindInternal)
	s2.SetStartTimestamp(pcommon.Timestamp(baseTime + 75*int64(time.Millisecond)))
	s2.SetEndTimestamp(pcommon.Timestamp(baseTime + 850*int64(time.Millisecond)))
	s2.Attributes().PutStr("child2.string", "child2-value")
	s2.Attributes().PutInt("child2.int", 99)
	s2.Attributes().PutDouble("child2.float", 1.414)
	s2.Status().SetCode(ptrace.StatusCodeOk)

	// Grandchild
	s3 := spans.AppendEmpty()
	s3.SetTraceID(traceID)
	s3.SetSpanID(grandchildSpanID)
	s3.SetParentSpanID(childSpanID)
	s3.SetName("grandchild-operation")
	s3.SetKind(ptrace.SpanKindInternal)
	s3.SetStartTimestamp(pcommon.Timestamp(baseTime + 200*int64(time.Millisecond)))
	s3.SetEndTimestamp(pcommon.Timestamp(baseTime + 700*int64(time.Millisecond)))
	s3.Attributes().PutStr("grandchild.string", "grandchild-value")
	s3.Attributes().PutInt("grandchild.int", 123)
	s3.Attributes().PutDouble("grandchild.float", 2.236)
	s3.Status().SetCode(ptrace.StatusCodeOk)

	// Great-grandchild
	s4 := spans.AppendEmpty()
	s4.SetTraceID(traceID)
	s4.SetSpanID(greatGrandchildSpanID)
	s4.SetParentSpanID(grandchildSpanID)
	s4.SetName("great-grandchild-operation")
	s4.SetKind(ptrace.SpanKindInternal)
	s4.SetStartTimestamp(pcommon.Timestamp(baseTime + 250*int64(time.Millisecond)))
	s4.SetEndTimestamp(pcommon.Timestamp(baseTime + 600*int64(time.Millisecond)))
	s4.Attributes().PutStr("great-grandchild.string", "great-grandchild-value")
	s4.Attributes().PutInt("great-grandchild.int", 456)
	s4.Status().SetCode(ptrace.StatusCodeError)
	s4.Status().SetMessage("deep operation failed")

	// Child2-child
	s5 := spans.AppendEmpty()
	s5.SetTraceID(traceID)
	s5.SetSpanID(child2ChildSpanID)
	s5.SetParentSpanID(child2SpanID)
	s5.SetName("child2-child-operation")
	s5.SetKind(ptrace.SpanKindInternal)
	s5.SetStartTimestamp(pcommon.Timestamp(baseTime + 150*int64(time.Millisecond)))
	s5.SetEndTimestamp(pcommon.Timestamp(baseTime + 750*int64(time.Millisecond)))
	s5.Attributes().PutStr("child2-child.string", "child2-child-value")
	s5.Attributes().PutInt("child2-child.int", 789)
	s5.Status().SetCode(ptrace.StatusCodeOk)

	// Orphaned span
	s6 := spans.AppendEmpty()
	s6.SetTraceID(traceID)
	s6.SetSpanID(orphanedSpanID)
	s6.SetParentSpanID(nonExistentParent)
	s6.SetName("orphaned-operation")
	s6.SetKind(ptrace.SpanKindInternal)
	s6.SetStartTimestamp(pcommon.Timestamp(baseTime + 100*int64(time.Millisecond)))
	s6.SetEndTimestamp(pcommon.Timestamp(baseTime + 800*int64(time.Millisecond)))
	s6.Attributes().PutStr("orphaned.string", "orphaned-value")
	s6.Status().SetCode(ptrace.StatusCodeUnset)

	// Orphaned child
	s7 := spans.AppendEmpty()
	s7.SetTraceID(traceID)
	s7.SetSpanID(orphanedChildSpanID)
	s7.SetParentSpanID(orphanedSpanID)
	s7.SetName("orphaned-child-operation")
	s7.SetKind(ptrace.SpanKindInternal)
	s7.SetStartTimestamp(pcommon.Timestamp(baseTime + 120*int64(time.Millisecond)))
	s7.SetEndTimestamp(pcommon.Timestamp(baseTime + 750*int64(time.Millisecond)))
	s7.Attributes().PutStr("orphaned-child.string", "orphaned-child-value")
	s7.Attributes().PutInt("orphaned-child.int", 555)
	s7.Status().SetCode(ptrace.StatusCodeOk)

	// Orphaned grandchild
	s8 := spans.AppendEmpty()
	s8.SetTraceID(traceID)
	s8.SetSpanID(orphanedGrandchildSpanID)
	s8.SetParentSpanID(orphanedChildSpanID)
	s8.SetName("orphaned-grandchild-operation")
	s8.SetKind(ptrace.SpanKindInternal)
	s8.SetStartTimestamp(pcommon.Timestamp(baseTime + 140*int64(time.Millisecond)))
	s8.SetEndTimestamp(pcommon.Timestamp(baseTime + 700*int64(time.Millisecond)))
	s8.Attributes().PutStr("orphaned-grandchild.string", "orphaned-grandchild-value")
	s8.Attributes().PutInt("orphaned-grandchild.int", 777)
	s8.Status().SetCode(ptrace.StatusCodeError)
	s8.Status().SetMessage("orphaned operation failed")

	return tr
}

// TestSpans_ServiceNameDenormStaysConsistent pins down the contract
// that the spans.service_name column (added as a hot index target for
// "filter by service" queries) stays in sync with its source-of-truth
// resource attribute. We ingest a mix of spans -- some with
// service.name set, some without -- and assert column-vs-attribute
// equality on every row. A single mismatch means either the ingest
// path forgot to write the column, or the resource attribute row was
// dropped, both of which would silently break service filtering.
func TestSpans_ServiceNameDenormStaysConsistent(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	traces, _, _, _ := buildTracesForSummaryOrdering(baseTime)

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	require.NoError(t, err)

	// For every span row, service_name must equal the value the resource
	// actually carries.
	//
	// The path to the source of truth changed: there is no longer a
	// span-keyed resource attribute row to left-join. It resolves through
	// resource_id -> resources.attribute_ids -> attributes, which the
	// attr_value macro does in one step. The inner join on resources is safe
	// because spans.resource_id is NOT NULL with an FK -- a span without a
	// resource cannot exist -- so an inner join here cannot hide a row the
	// way it would have under the old nullable-owner shape. attr_value
	// returns NULL when the key is absent, hence the coalesce: spans whose
	// resource has no service.name must carry '' in the column.
	var mismatches int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select count(*) from spans s
			join resources r on r.id = s.resource_id
			where s.service_name <> coalesce(attr_value(r.attribute_ids, 'service.name'), '')
		`).Scan(&mismatches)
	}))
	assert.Equal(t, 0, mismatches,
		"spans.service_name must equal the source resource attribute (or '' when absent)")

	// Guard the inner join: if resource_id ever stopped resolving, the query
	// above would return 0 mismatches over 0 rows and pass vacuously.
	var joined int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select count(*) from spans s join resources r on r.id = s.resource_id
		`).Scan(&joined)
	}))
	assert.Equal(t, countRows(t, s, ctx, "select count(*) from spans"), joined,
		"every span must resolve to a resource row, or the check above passes vacuously")
	assert.Greater(t, joined, 0)
}
