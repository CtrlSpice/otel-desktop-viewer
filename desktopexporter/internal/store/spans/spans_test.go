package spans_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "")
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

func countRows(t *testing.T, db *sql.DB, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
	return n
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
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000003"
}

// searchTracesAll returns SearchTraces with a wide time range and nil query to get "all" summaries.
func searchTracesAll(t *testing.T, s *store.Store, ctx context.Context) []traceSummaryJSON {
	t.Helper()
	const maxNano = 1<<63 - 1
	raw, err := spans.SearchTraces(ctx, s.DB(), 0, maxNano, nil)
	assert.NoError(t, err)
	var summaries []traceSummaryJSON
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	return summaries
}

type traceSummaryJSON struct {
	TraceID        string        `json:"traceID"`
	RootSpan       *rootSpanJSON `json:"rootSpan"`
	SpanCount      float64       `json:"spanCount"` // JSON number
	ErrorCount     float64       `json:"errorCount"`
	ExceptionCount float64       `json:"exceptionCount"`
}

type rootSpanJSON struct {
	ServiceName string `json:"serviceName"`
	Name        string `json:"name"`
	StartTime   int64  `json:"startTime"`
	EndTime     int64  `json:"endTime"`
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
}

// TestTraceNotFound verifies error handling for non-existent trace IDs.
func TestTraceNotFound(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	_, err := spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", nil)
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

// TestClearTraces verifies that all traces can be cleared from the store, including child rows.
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
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from events"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from links"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"), 0)

	err = spans.Clear(ctx, s.DB())
	assert.NoError(t, err)

	summaries = searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from events"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from links"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"))
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
	testTraceID := "00000000-0000-0000-0000-000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err, "failed to ingest test trace")

	t.Run("TraceHierarchicalStructure", func(t *testing.T) {
		raw, err := spans.SearchSpans(ctx, s.DB(), testTraceID, nil)
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
		_, err := spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, spans.ErrTraceIDNotFound)
	})

	t.Run("SearchSpansAcceptsTraceIDWithoutHyphens", func(t *testing.T) {
		raw, err := spans.SearchSpans(ctx, s.DB(), "00000000000000000000000000000099", nil)
		assert.NoError(t, err, "SearchSpans with 32-char hex trace ID should succeed")
		got := getTraceTraceID(t, raw)
		assert.True(t, got == "00000000-0000-0000-0000-000000000099" || got == "00000000000000000000000000000099",
			"response traceID should be the same logical UUID (got %q)", got)
	})

	t.Run("AttributeDiscovery", func(t *testing.T) {
		now := time.Now().UnixNano()
		start := now - 24*int64(time.Hour)
		end := now + 24*int64(time.Hour)
		raw, err := spans.GetTraceAttributes(ctx, s.DB(), start, end)
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
	testTraceID := "00000000-0000-0000-0000-000000000099"
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
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
		raw, err := spans.SearchTraces(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		summaries := parseSummaries(raw)
		assert.NotEmpty(t, summaries)
		assert.Equal(t, testTraceID, summaries[0].TraceID)
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

	testTraceID := "00000000-0000-0000-0000-000000000099"
	raw, err := spans.SearchSpans(ctx, s.DB(), testTraceID, nil)
	assert.NoError(t, err)
	assert.Equal(t, batchSize, getTraceSpansCount(t, raw))

	// Assert attributes flushed: span 1 (index 0), span 50 (index 49), span 51 (index 50)
	// SpanID for index i is (i+1) as 16-char hex; UUID format is 8-4-4-4-12.
	for _, spanIndex := range []int{0, 49, 50} {
		spanIDHex := fmt.Sprintf("%016x", spanIndex+1)
		spanUUID := "00000000-0000-0000-0000-" + spanIDHex[4:]
		attrCount := countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id = ? and scope = 'span' and key in ('span.index', 'flush_test')", spanUUID)
		assert.GreaterOrEqual(t, attrCount, 2, "span %d should have span.index and flush_test attributes", spanIndex)
	}
	// Resource/scope attributes on first span
	span1UUID := "00000000-0000-0000-0000-000000000001"
	resAttr := countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id = ? and scope = 'resource'", span1UUID)
	scopeAttr := countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id = ? and scope = 'scope'", span1UUID)
	assert.GreaterOrEqual(t, resAttr, 1)
	assert.GreaterOrEqual(t, scopeAttr, 1)
}

// TestDeleteSpanByID verifies that a single span can be deleted by its SpanID, including child rows.
func TestDeleteSpanByID(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	raw, err := spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000099", nil)
	assert.NoError(t, err)
	assert.Equal(t, 9, getTraceSpansCount(t, raw))

	spanUUID := "00000000-0000-0000-0000-000000000001"
	eventsBefore := countRows(t, s.DB(), ctx, "select count(*) from events where span_id = ?", spanUUID)
	linksBefore := countRows(t, s.DB(), ctx, "select count(*) from links where span_id = ?", spanUUID)
	attrsBefore := countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id = ?", spanUUID)
	assert.Greater(t, eventsBefore+linksBefore+attrsBefore, 0, "root span should have child rows")

	err = spans.DeleteSpanByID(ctx, s.DB(), spanUUID)
	assert.NoError(t, err)

	raw, err = spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000099", nil)
	assert.NoError(t, err)
	assert.Equal(t, 8, getTraceSpansCount(t, raw))

	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from events where span_id = ?", spanUUID))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from links where span_id = ?", spanUUID))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id = ?", spanUUID))
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

	raw, err := spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000099", nil)
	assert.NoError(t, err)
	assert.Equal(t, 9, getTraceSpansCount(t, raw))

	deletedIDs := []any{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000003",
	}
	attrsBefore := countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id in (?, ?, ?)", deletedIDs...)
	assert.Greater(t, attrsBefore, 0, "deleted spans should have attributes")

	err = spans.DeleteSpansByIDs(ctx, s.DB(), deletedIDs)
	assert.NoError(t, err)

	raw, err = spans.SearchSpans(ctx, s.DB(), "00000000-0000-0000-0000-000000000099", nil)
	assert.NoError(t, err)
	assert.Equal(t, 6, getTraceSpansCount(t, raw))

	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from events where span_id in (?, ?, ?)", deletedIDs...))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from links where span_id in (?, ?, ?)", deletedIDs...))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id in (?, ?, ?)", deletedIDs...))
}

// TestDeleteSpansByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteSpansByIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := spans.DeleteSpansByIDs(ctx, s.DB(), []any{})
	assert.NoError(t, err)
}

// TestDeleteSpansByTraceID verifies that all spans for a trace are deleted, including child rows.
func TestDeleteSpansByTraceID(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	testTraceID := "00000000-0000-0000-0000-000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	summaries := searchTracesAll(t, s, ctx)
	assert.Len(t, summaries, 1)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from events"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from links"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"), 0)

	err = spans.DeleteSpansByTraceID(ctx, s.DB(), testTraceID)
	assert.NoError(t, err)

	summaries = searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from events"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from links"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"))
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

	raw, err := spans.SearchSpans(ctx, s.DB(), "00000000000000000000000000000099", nil)
	assert.NoError(t, err, "SearchSpans with 32-char hex trace ID should succeed")
	assert.NotEmpty(t, raw)
	got := getTraceTraceID(t, raw)
	assert.True(t, got == "00000000-0000-0000-0000-000000000099" || got == "00000000000000000000000000000099",
		"response traceID should be the same logical UUID (got %q)", got)
}

// TestDeleteSpansByTraceIDs verifies that spans for multiple traces are deleted, including child rows.
func TestDeleteSpansByTraceIDs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	traces := createTestTracePdata()
	testTraceID := "00000000-0000-0000-0000-000000000099"
	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	})
	assert.NoError(t, err)

	summaries := searchTracesAll(t, s, ctx)
	assert.Len(t, summaries, 1)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from events"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from links"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"), 0)

	err = spans.DeleteSpansByTraceIDs(ctx, s.DB(), []any{testTraceID})
	assert.NoError(t, err)

	summaries = searchTracesAll(t, s, ctx)
	assert.Empty(t, summaries)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from events"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from links"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where span_id is not null"))
}

// TestDeleteSpansByTraceIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteSpansByTraceIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := spans.DeleteSpansByTraceIDs(ctx, s.DB(), []any{})
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
