package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// tracesWithDuplicateSpanID builds one trace of n spans whose last `dupes` span
// ids repeat earlier ones. Every span carries an event and a link, so the child
// rows are exercised alongside their parent.
func tracesWithDuplicateSpanID(n, dupes int) ptrace.Traces {
	baseTime := time.Now().UnixNano()
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "dupe-service")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("dupe-scope")

	traceID := storetest.MustDecodeTraceID("000000000000000000000000000000aa")
	ids := make([]int, 0, n)
	for i := 1; i <= n-dupes; i++ {
		ids = append(ids, i)
	}
	for i := 1; i <= dupes; i++ {
		ids = append(ids, i) // repeats of ids already in this batch
	}

	for _, k := range ids {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(traceID)
		s.SetSpanID(storetest.MustDecodeSpanID(fmt.Sprintf("%016x", k)))
		s.SetName(fmt.Sprintf("span-%d", k))
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.Timestamp(baseTime))
		s.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))
		s.Attributes().PutStr("span.n", fmt.Sprintf("%d", k))

		e := s.Events().AppendEmpty()
		e.SetName(fmt.Sprintf("event-%d", k))
		e.SetTimestamp(pcommon.Timestamp(baseTime))
		e.Attributes().PutStr("event.n", fmt.Sprintf("%d", k))

		l := s.Links().AppendEmpty()
		l.SetTraceID(traceID)
		l.SetSpanID(storetest.MustDecodeSpanID("00000000000000ff"))
		l.Attributes().PutStr("link.n", fmt.Sprintf("%d", k))
	}
	return tr
}

func ingestReport(t *testing.T, s *store.Store, tr ptrace.Traces) (ingest.Rejected, error) {
	t.Helper()
	var rep ingest.Rejected
	err := s.WithConn(func(conn driver.Conn) error {
		var iErr error
		rep, iErr = spans.IngestReport(t.Context(), conn, tr)
		return iErr
	})
	return rep, err
}

// TestIngest_OneDuplicateDoesNotCostTheBatch verifies that one duplicate span
// is rejected without discarding valid spans in the same appender buffer.
func TestIngest_OneDuplicateDoesNotCostTheBatch(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	rep, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 1))
	require.NoError(t, err, "a duplicate row must not fail the batch")

	assert.Equal(t, 1, rep.Count(), "exactly one span should have been rejected")
	require.Error(t, rep.Reason())
	assert.ErrorIs(t, rep.Reason(), spans.ErrSpanAlreadyStored,
		"the report should say why, so a sender can tell its ids are being reused")

	assert.Equal(t, 599, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 599, storetest.CountRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 599, storetest.CountRows(t, s, ctx, "select count(*) from links"))
}

func TestIngest_EmptySpanIDIsRefused(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	tr := ptrace.NewTraces()
	ss := tr.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := storetest.MustDecodeTraceID("000000000000000000000000000000ab")
	for _, tc := range []struct {
		name   string
		spanID string
	}{
		{name: "invalid"},
		{name: "valid", spanID: "0000000000000001"},
	} {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		if tc.spanID != "" {
			span.SetSpanID(storetest.MustDecodeSpanID(tc.spanID))
		}
		span.SetName(tc.name)
	}

	rep, err := ingestReport(t, s, tr)
	require.NoError(t, err)
	require.Equal(t, 1, rep.Count())
	require.ErrorIs(t, rep.Reason(), spans.ErrInvalidSpanID)
	require.Equal(t, 1, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
	require.Equal(t, 0, storetest.CountRows(t, s, ctx, "select count(*) from spans where span_id = 0"))
}

// Several bad spans, still only they are lost.
//
// DuckDB names only the first violation it hits, so the count cannot come from
// reading the error -- it comes from narrowing until each bad row stands alone.
func TestIngest_ManyDuplicatesCostOnlyThemselves(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	rep, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 25))
	require.NoError(t, err)

	assert.Equal(t, 25, rep.Count())
	assert.Equal(t, 575, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
}

// A resent batch stores nothing new and is not an error. This is the reported
// case: a sender re-emitting a trace the store already holds.
func TestIngest_ResendRejectedWithoutFailing(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)
	require.Equal(t, 9, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
	events := storetest.CountRows(t, s, ctx, "select count(*) from events")

	rep, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err, "a resent batch must not be an error")
	assert.Equal(t, 9, rep.Count(), "every span in a resend is already stored")
	assert.Equal(t, 9, storetest.CountRows(t, s, ctx, "select count(*) from spans"),
		"a resend must not duplicate rows")
	assert.Equal(t, events, storetest.CountRows(t, s, ctx, "select count(*) from events"),
		"a resend must not duplicate child rows either")
}

// Rejecting rows must leave no children behind and must not wedge the
// connection. Ingest owns one connection for the life of the process, so a
// transaction left open would fail every batch after this one.
func TestIngest_RejectionsLeaveNoChildrenAndNoOpenTransaction(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	for i := range 3 {
		_, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 5))
		require.NoError(t, err, "batch %d", i)
	}

	assert.Zero(t, storetest.CountRows(t, s, ctx, `
		select count(*) from events e
		where not exists (select 1 from spans s where s.span_id = e.span_id)
	`), "orphaned events")
	assert.Zero(t, storetest.CountRows(t, s, ctx, `
		select count(*) from links l
		where not exists (select 1 from spans s where s.span_id = l.span_id)
	`), "orphaned links")

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err, "a later batch must still ingest")
}

// Rejected spans must not remove payloads owned by accepted spans.
func TestIngest_AcceptedSpansRetainOwnedPayloads(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 25))
	require.NoError(t, err)
	_, err = ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)

	assert.Zero(t, storetest.CountRows(t, s, ctx, `
		select count(*)
		from spans where attributes is null or resource is null or scope is null
	`), "accepted spans must retain complete owned payloads")
}

// When a batch repeats an id, the first occurrence is the one that lands.
// Bisection attempts ranges in order to make this true rather than incidental,
// and dedupe keeps the first ordinal it sees for the same reason.
func TestIngest_FirstOccurrenceOfARepeatedIDWins(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "first-wins")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("sc")
	traceID := storetest.MustDecodeTraceID("000000000000000000000000000000cc")

	for _, name := range []string{"first", "second"} {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(storetest.MustDecodeSpanID("0000000000000001")) // same id twice
		sp.SetName(name)
		sp.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		sp.SetEndTimestamp(pcommon.Timestamp(time.Now().UnixNano() + 1))
	}

	rep, err := ingestReport(t, s, tr)
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Count())
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx,
		"select count(*) from spans where name = 'first'"),
		"the first occurrence should be the one stored")
}

// A batch mixing stored and new spans keeps the new ones.
func TestIngest_MixedResendKeepsTheNewSpans(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, tracesWithDuplicateSpanID(100, 0))
	require.NoError(t, err)
	require.Equal(t, 100, storetest.CountRows(t, s, ctx, "select count(*) from spans"))

	// 150 spans, ids 1..150: the first 100 are already stored.
	rep, err := ingestReport(t, s, tracesWithDuplicateSpanID(150, 0))
	require.NoError(t, err)
	assert.Equal(t, 100, rep.Count(), "the overlap should be reported")
	assert.Equal(t, 150, storetest.CountRows(t, s, ctx, "select count(*) from spans"),
		"the 50 new spans should have landed")
}

// Each span retains the complete shared payload with the same grouping identity.
func TestIngestRepeatsOwnedPayloadAcrossSpans(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	rs.Resource().Attributes().PutStr("host.name", "pod-a")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("otelhttp")
	ss.Scope().SetVersion("1.2.0")

	const n = 200
	for i := 0; i < n; i++ {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		var sid [8]byte
		for b := 0; b < 8; b++ {
			sid[b] = byte((i + 1) >> (b * 8))
		}
		sp.SetSpanID(sid)
		sp.SetName("GET /cart")
		now := time.Now()
		sp.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
		sp.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))
		sp.Attributes().PutStr("http.request.method", "GET")
		sp.Attributes().PutInt("http.response.status_code", 200)
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, td)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var spanCount, resourceCount, scopeCount, attrCount int
		require.NoError(t, db.QueryRow(`select count(*) from spans`).Scan(&spanCount))
		require.NoError(t, db.QueryRow(`select count(distinct resource.id) from spans`).Scan(&resourceCount))
		require.NoError(t, db.QueryRow(`select count(distinct scope.id) from spans`).Scan(&scopeCount))
		require.NoError(t, db.QueryRow(`select sum(len(variant_keys(attributes)) + len(variant_keys(resource.attributes))) from spans`).Scan(&attrCount))

		assert.Equal(t, n, spanCount)
		assert.Equal(t, 1, resourceCount, "200 spans, one resource")
		assert.Equal(t, 1, scopeCount)
		// 2 resource attrs + 2 span attrs; the scope has none.
		assert.Equal(t, 4*n, attrCount, "every span owns all received attributes")

		var dangling int
		require.NoError(t, db.QueryRow(`
			select count(*) from spans where resource.attributes['service.name']::varchar <> 'checkout'
			or resource.attributes['host.name']::varchar <> 'pod-a'`).Scan(&dangling))
		assert.Zero(t, dangling)

		var mismatched int
		require.NoError(t, db.QueryRow(
			`select count(*) from spans where attributes['http.request.method']::varchar <> 'GET'
			or attributes['http.response.status_code']::bigint <> 200`).Scan(&mismatched))
		assert.Zero(t, mismatched)
		return nil
	}))
}

type rejectionRow struct {
	kind        string
	occurrences int64
	samples     int64
}

func readRejections(t *testing.T, s *store.Store) []rejectionRow {
	t.Helper()
	out, err := storetest.Read(s, func(db *sql.DB) ([]rejectionRow, error) {
		rows, err := db.Query(`select kind, occurrences, len(samples)
			from ingest_rejections order by kind`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var got []rejectionRow
		for rows.Next() {
			var r rejectionRow
			if err := rows.Scan(&r.kind, &r.occurrences, &r.samples); err != nil {
				return nil, err
			}
			got = append(got, r)
		}
		return got, rows.Err()
	})
	require.NoError(t, err)
	return out
}

// A resend records one row saying how many spans it refused, with a span id to
// link to -- not one row per refused span.
func TestRejections_ResendRecordsOneAggregateRow(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)
	assert.Empty(t, readRejections(t, s), "a clean batch records nothing")

	_, err = ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)

	got := readRejections(t, s)
	require.Len(t, got, 1, "one row per (signal, kind), not per span")
	assert.Equal(t, "span_already_stored", got[0].kind)
	assert.EqualValues(t, 9, got[0].occurrences)
	assert.EqualValues(t, 9, got[0].samples, "one sample per distinct refused span")
}

// Repeating the resend accumulates into the same row rather than growing the
// table, which is what keeps a replay loop from bloating the database.
func TestRejections_RepeatedResendsAccumulateInOneRow(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	for range 4 {
		_, err := ingestReport(t, s, createTestTracePdata())
		require.NoError(t, err)
	}

	got := readRejections(t, s)
	require.Len(t, got, 1, "a replay loop must not grow the table")
	assert.EqualValues(t, 27, got[0].occurrences, "3 resends of 9 spans")
	assert.EqualValues(t, 9, got[0].samples,
		"the same nine identities re-refused must dedupe, not fill the bound")
}

func TestRejections_EmptySpanIDIsRefusedWithoutLinkableSample(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(storetest.MustDecodeTraceID("000000000000000000000000000000ac"))

	rep, err := ingestReport(t, s, traces)
	require.NoError(t, err)
	require.ErrorIs(t, rep.Reason(), spans.ErrInvalidSpanID)

	got := readRejections(t, s)
	require.Len(t, got, 1)
	assert.Equal(t, "span_refused", got[0].kind)
	assert.EqualValues(t, 1, got[0].occurrences)
	assert.Zero(t, got[0].samples, "an empty ID must not become an all-zero route")
}

// traceReusingSpanIDs builds one trace whose spans use ids 1 and 2, with an
// event and a link on each, named after the trace so they can be told apart.
func traceReusingSpanIDs(traceHex, label string) ptrace.Traces {
	base := time.Now().UnixNano()
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", label)
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("sc")
	traceID := storetest.MustDecodeTraceID(traceHex)

	for _, n := range []string{"0000000000000001", "0000000000000002"} {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(storetest.MustDecodeSpanID(n))
		sp.SetName(label + "-" + n)
		sp.SetStartTimestamp(pcommon.Timestamp(base))
		sp.SetEndTimestamp(pcommon.Timestamp(base + int64(time.Second)))

		e := sp.Events().AppendEmpty()
		e.SetName(label + "-event")
		e.SetTimestamp(pcommon.Timestamp(base))

		l := sp.Links().AppendEmpty()
		l.SetTraceID(storetest.MustDecodeTraceID("000000000000000000000000000000ff"))
		l.SetSpanID(storetest.MustDecodeSpanID("00000000000000ff"))
	}
	return tr
}

// Two traces may legitimately use the same span ids.
//
// Span ids are only required to be unique within a trace -- global uniqueness
// is a property of random generation, not a guarantee. Keying spans on span_id
// alone rejected the second trace outright, which made a conformant sender look
// like it was reusing ids.
func TestCompositeKey_TwoTracesMayShareSpanIDs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	repA, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000a1", "alpha"))
	require.NoError(t, err)
	assert.Zero(t, repA.Count())

	repB, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000b2", "bravo"))
	require.NoError(t, err)
	assert.Zero(t, repB.Count(), "a second trace reusing span ids is not a duplicate")

	assert.Equal(t, 4, storetest.CountRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 4, storetest.CountRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 4, storetest.CountRows(t, s, ctx, "select count(*) from links"))
}

// Fetching one of those traces must not pull in the other's children, which is
// the contamination the composite key makes possible if a join forgets the
// trace.
func TestCompositeKey_TraceFetchDoesNotBorrowAnotherTracesChildren(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000a1", "alpha"))
	require.NoError(t, err)
	_, err = ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000b2", "bravo"))
	require.NoError(t, err)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceView(ctx, db, "00000000-0000-0000-0000-0000000000a1", nil)
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	body := string(raw)
	assert.Contains(t, body, "alpha-event")
	assert.NotContains(t, body, "bravo-event",
		"events from the other trace leaked in through a span id match")
	assert.Equal(t, 2, len(got["spans"].([]any)))
}

func TestSpanTimestampsRoundTripAcrossUint64Range(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	values := []uint64{0, 1<<63 - 1, 1 << 63, ^uint64(0)}

	data := ptrace.NewTraces()
	ss := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID{15: 1}
	for i, timestamp := range values {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
		span.SetName("boundary")
		span.SetStartTimestamp(pcommon.Timestamp(timestamp))
		span.SetEndTimestamp(pcommon.Timestamp(timestamp))
		event := span.Events().AppendEmpty()
		event.SetName("boundary")
		event.SetTimestamp(pcommon.Timestamp(timestamp))
	}
	skewed := ss.Spans().AppendEmpty()
	skewed.SetTraceID(traceID)
	skewed.SetSpanID(pcommon.SpanID{7: 5})
	skewed.SetName("skewed")
	skewed.SetStartTimestamp(pcommon.Timestamp(^uint64(0)))
	skewed.SetEndTimestamp(0)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select s.start_time, s.end_time, e.timestamp
			from spans s join events e using (trace_id, span_id)
			order by s.span_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, want := range values {
			require.True(t, rows.Next())
			var start, end, event uint64
			require.NoError(t, rows.Scan(&start, &end, &event))
			require.Equal(t, want, start)
			require.Equal(t, want, end)
			require.Equal(t, want, event)
		}
		return rows.Err()
	}))

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraceSummaries(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Equal(t, "0", summaries[0]["startTime"])
	require.Equal(t, "18446744073709551615", summaries[0]["durationNs"])

	raw, err = storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceView(ctx, db, hex.EncodeToString(traceID[:]), nil)
	})
	require.NoError(t, err)
	var trace struct {
		TraceStart string `json:"traceStart"`
		Spans      []struct {
			SpanData struct {
				Name  string `json:"name"`
				Start string `json:"start"`
				Dur   string `json:"dur"`
			} `json:"spanData"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal(raw, &trace))
	require.Equal(t, "0", trace.TraceStart)
	for _, node := range trace.Spans {
		if node.SpanData.Name == "skewed" {
			require.Equal(t, "18446744073709551615", node.SpanData.Start)
			require.Equal(t, "-18446744073709551615", node.SpanData.Dur)
			return
		}
	}
	t.Fatal("skewed span not returned")
}

func TestSpanNumericEnumsRoundTripAndRemainSearchable(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := [16]byte{15: 42}
	cases := []struct {
		kind        ptrace.SpanKind
		status      ptrace.StatusCode
		kindLabel   string
		statusLabel string
	}{
		{ptrace.SpanKindUnspecified, ptrace.StatusCodeUnset, "Unspecified", "Unset"},
		{ptrace.SpanKindServer, ptrace.StatusCodeError, "Server", "Error"},
		{ptrace.SpanKind(99), ptrace.StatusCode(99), "Unknown (99)", "Unknown (99)"},
		{ptrace.SpanKind(-1), ptrace.StatusCode(-1), "Unknown (-1)", "Unknown (-1)"},
		{ptrace.SpanKind(math.MinInt32), ptrace.StatusCode(math.MaxInt32), "Unknown (-2147483648)", "Unknown (2147483647)"},
	}

	data := ptrace.NewTraces()
	ss := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i, tc := range cases {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID([8]byte{7: byte(i + 1)})
		span.SetName(tc.kindLabel)
		span.SetKind(tc.kind)
		span.Status().SetCode(tc.status)
		span.SetStartTimestamp(pcommon.Timestamp(i + 1))
		span.SetEndTimestamp(pcommon.Timestamp(i + 2))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select kind, status_code from spans order by span_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, tc := range cases {
			require.True(t, rows.Next())
			var kind, status int32
			require.NoError(t, rows.Scan(&kind, &status))
			assert.Equal(t, int32(tc.kind), kind)
			assert.Equal(t, int32(tc.status), status)
		}
		return rows.Err()
	}))

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceView(ctx, db, "0000000000000000000000000000002a", nil)
	})
	require.NoError(t, err)
	var detail struct {
		Spans []struct {
			SpanData struct {
				KindCode        int32  `json:"kindCode"`
				Kind            string `json:"kind"`
				StatusCodeValue int32  `json:"statusCodeValue"`
				StatusCode      string `json:"statusCode"`
			} `json:"spanData"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal(raw, &detail))
	require.Len(t, detail.Spans, len(cases))
	for i, tc := range cases {
		assert.Equal(t, int32(tc.kind), detail.Spans[i].SpanData.KindCode)
		assert.Equal(t, tc.kindLabel, detail.Spans[i].SpanData.Kind)
		assert.Equal(t, int32(tc.status), detail.Spans[i].SpanData.StatusCodeValue)
		assert.Equal(t, tc.statusLabel, detail.Spans[i].SpanData.StatusCode)
	}

	for _, tc := range []struct {
		field    string
		typeName string
		value    any
	}{
		{"kind", "string", "Server"},
		{"kindCode", "int64", int64(99)},
		{"statusCode", "string", "Error"},
		{"statusCodeValue", "int64", int64(-1)},
	} {
		query := &search.QueryNode{ID: tc.field, Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: tc.field, Type: tc.typeName, SearchScope: "field"},
			FieldOperator: "=", Value: fmt.Sprint(tc.value),
		}}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraceSummaries(ctx, db, store.BoundedTimeRange(0, 10), query)
		})
		require.NoError(t, err)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)
	}

	raw, err = storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraceSummaries(ctx, db, store.BoundedTimeRange(0, 10), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, float64(1), summaries[0]["errorCount"], "only status code 2 is an error")
}
