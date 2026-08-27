package spans_test

import (
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// tracesWithDuplicateSpanID builds one trace of n spans whose last span reuses
// the first span's id. n is deliberately a caller's choice: a batch larger than
// flushIntervalSpans crosses an explicit flush, which is where partial commits
// come from.
func tracesWithDuplicateSpanID(n int) ptrace.Traces {
	baseTime := time.Now().UnixNano()
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "dupe-service")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("dupe-scope")

	traceID := mustDecodeTraceID("000000000000000000000000000000aa")
	ids := make([]int, 0, n)
	for i := 1; i < n; i++ {
		ids = append(ids, i)
	}
	ids = append(ids, 1) // the duplicate, last
	for _, n := range ids {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(traceID)
		s.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", n)))
		s.SetName(fmt.Sprintf("span-%d", n))
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.Timestamp(baseTime))
		s.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))
		s.Attributes().PutStr("span.n", fmt.Sprintf("%d", n))

		e := s.Events().AppendEmpty()
		e.SetName(fmt.Sprintf("event-%d", n))
		e.SetTimestamp(pcommon.Timestamp(baseTime))
		e.Attributes().PutStr("event.n", fmt.Sprintf("%d", n))

		l := s.Links().AppendEmpty()
		l.SetTraceID(traceID)
		l.SetSpanID(mustDecodeSpanID("00000000000000ff"))
		l.Attributes().PutStr("link.n", fmt.Sprintf("%d", n))
	}
	return tr
}

// A batch that fails partway must leave nothing behind.
//
// The appenders flush every flushIntervalSpans (500) spans, and each flush used
// to be its own commit. So a batch of more than 500 spans that failed later --
// on a duplicate span id, say -- left the first 500 durable while the call
// returned an error. The collector then retried the whole batch, whose first
// 500 spans now collided with the copies we had already kept, and the retry
// failed for a reason the first attempt created. That is a batch that can never
// succeed, and it needs a batch larger than one flush interval to reproduce.
func TestIngest_FailedBatchOver500SpansCommitsNothing(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, tracesWithDuplicateSpanID(600), s.FlushedIDs())
	})
	require.Error(t, err, "a duplicate span id must fail the batch")

	assert.Zero(t, countRows(t, s, ctx, "select count(*) from spans"),
		"spans from a flush before the failure survived, so a retry of this batch can never succeed")
	assert.Zero(t, countRows(t, s, ctx, "select count(*) from events"))
	assert.Zero(t, countRows(t, s, ctx, "select count(*) from links"))

	// The retry the collector would make must now work.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, createTestTracePdata(), s.FlushedIDs())
	}), "a clean batch after a failed one must still ingest")
}

// The dictionary cache must not keep claiming rows a rollback removed.
//
// FlushedIDs marks ids as soon as their insert succeeds, and the insert does
// succeed -- it is the appender flush afterwards that fails. Without
// invalidation the next batch would skip re-inserting those rows and its spans
// would carry uuid[] references into nothing, which no foreign key catches.
func TestIngest_FailedBatchDoesNotPoisonDictionaryCache(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, tracesWithDuplicateSpanID(600), s.FlushedIDs())
	})
	require.Error(t, err)

	// A clean batch reusing the same resource, scope and attribute values must
	// still write its dictionary rows.
	clean := createTestTracePdata()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, clean, s.FlushedIDs())
	}))

	assert.NotZero(t, countRows(t, s, ctx, "select count(*) from spans"))
	assert.NotZero(t, countRows(t, s, ctx, "select count(*) from attributes"),
		"dictionary rows must be rewritten after a rollback")
	assert.NotZero(t, countRows(t, s, ctx, "select count(*) from resources"))

	// Every attribute id a span references must resolve to a real row.
	dangling := countRows(t, s, ctx, `
		select count(*)
		from (select unnest(attribute_ids) as id from spans) x
		where not exists (select 1 from attributes a where a.id = x.id)
	`)
	assert.Zero(t, dangling, "spans reference dictionary rows that do not exist")
}
