package spans_test

import (
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
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

	traceID := mustDecodeTraceID("000000000000000000000000000000aa")
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
		s.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", k)))
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
		l.SetSpanID(mustDecodeSpanID("00000000000000ff"))
		l.Attributes().PutStr("link.n", fmt.Sprintf("%d", k))
	}
	return tr
}

func ingestReport(t *testing.T, s *store.Store, tr ptrace.Traces) (ingest.Rejected, error) {
	t.Helper()
	var rep ingest.Rejected
	err := s.WithConn(func(conn driver.Conn) error {
		var iErr error
		rep, iErr = spans.IngestReport(t.Context(), conn, tr, s.FlushedIDs())
		return iErr
	})
	return rep, err
}

// One bad span costs itself, not the batch.
//
// The appender only discovers a constraint violation at flush, and a failed
// flush discards its whole buffer, so a duplicate id used to throw away every
// good span beside it. 600 spans carrying one duplicate must store 599.
func TestIngest_OneDuplicateDoesNotCostTheBatch(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	rep, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 1))
	require.NoError(t, err, "a duplicate row must not fail the batch")

	assert.Equal(t, 1, rep.Count(), "exactly one span should have been rejected")
	require.Error(t, rep.Reason())
	assert.ErrorIs(t, rep.Reason(), spans.ErrSpanAlreadyStored,
		"the report should say why, so a sender can tell its ids are being reused")

	assert.Equal(t, 599, countRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 599, countRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 599, countRows(t, s, ctx, "select count(*) from links"))
}

func TestIngest_EmptySpanIDIsRefused(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	tr := ptrace.NewTraces()
	ss := tr.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := mustDecodeTraceID("000000000000000000000000000000ab")
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
			span.SetSpanID(mustDecodeSpanID(tc.spanID))
		}
		span.SetName(tc.name)
	}

	rep, err := ingestReport(t, s, tr)
	require.NoError(t, err)
	require.Equal(t, 1, rep.Count())
	require.ErrorIs(t, rep.Reason(), spans.ErrInvalidSpanID)
	require.Equal(t, 1, countRows(t, s, ctx, "select count(*) from spans"))
	require.Equal(t, 0, countRows(t, s, ctx, "select count(*) from spans where span_id = 0"))
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
	assert.Equal(t, 575, countRows(t, s, ctx, "select count(*) from spans"))
}

// A resent batch stores nothing new and is not an error. This is the reported
// case: a sender re-emitting a trace the store already holds.
func TestIngest_ResendRejectedWithoutFailing(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)
	require.Equal(t, 9, countRows(t, s, ctx, "select count(*) from spans"))
	events := countRows(t, s, ctx, "select count(*) from events")

	rep, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err, "a resent batch must not be an error")
	assert.Equal(t, 9, rep.Count(), "every span in a resend is already stored")
	assert.Equal(t, 9, countRows(t, s, ctx, "select count(*) from spans"),
		"a resend must not duplicate rows")
	assert.Equal(t, events, countRows(t, s, ctx, "select count(*) from events"),
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

	assert.Zero(t, countRows(t, s, ctx, `
		select count(*) from events e
		where not exists (select 1 from spans s where s.span_id = e.span_id)
	`), "orphaned events")
	assert.Zero(t, countRows(t, s, ctx, `
		select count(*) from links l
		where not exists (select 1 from spans s where s.span_id = l.span_id)
	`), "orphaned links")

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err, "a later batch must still ingest")
}

// Dictionary rows written for spans that were then rejected are harmless
// orphans, but a span that *was* written must never reference a missing one.
func TestIngest_NoDanglingAttributeReferences(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, tracesWithDuplicateSpanID(600, 25))
	require.NoError(t, err)
	_, err = ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)

	assert.Zero(t, countRows(t, s, ctx, `
		select count(*)
		from (select unnest(attribute_ids) as id from spans) x
		where not exists (select 1 from attributes a where a.id = x.id)
	`), "spans reference dictionary rows that do not exist")
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
	traceID := mustDecodeTraceID("000000000000000000000000000000cc")

	for _, name := range []string{"first", "second"} {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(mustDecodeSpanID("0000000000000001")) // same id twice
		sp.SetName(name)
		sp.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		sp.SetEndTimestamp(pcommon.Timestamp(time.Now().UnixNano() + 1))
	}

	rep, err := ingestReport(t, s, tr)
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Count())
	assert.Equal(t, 1, countRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 1, countRows(t, s, ctx,
		"select count(*) from spans where name = 'first'"),
		"the first occurrence should be the one stored")
}

// A batch mixing stored and new spans keeps the new ones.
func TestIngest_MixedResendKeepsTheNewSpans(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, tracesWithDuplicateSpanID(100, 0))
	require.NoError(t, err)
	require.Equal(t, 100, countRows(t, s, ctx, "select count(*) from spans"))

	// 150 spans, ids 1..150: the first 100 are already stored.
	rep, err := ingestReport(t, s, tracesWithDuplicateSpanID(150, 0))
	require.NoError(t, err)
	assert.Equal(t, 100, rep.Count(), "the overlap should be reported")
	assert.Equal(t, 150, countRows(t, s, ctx, "select count(*) from spans"),
		"the 50 new spans should have landed")
}
