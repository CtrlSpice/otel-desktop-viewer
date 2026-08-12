package store

// Referential integrity for the attribute dictionary.
//
// Every owner -- spans, events, links, logs, datapoints, exemplars, resources,
// scopes -- holds a uuid[] of attribute ids, and DuckDB cannot put a foreign key
// into a LIST. So the one relationship the schema most depends on is the one
// relationship it cannot enforce. Under the previous schema this was an FK;
// these tests are what replaced it.
//
// The failure mode is why they exist: a dangling reference produces no error
// anywhere. attrs_json simply joins nothing, the attribute disappears from the
// UI, and the first sign of trouble is a user asking where their data went.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// ownerArrays is every table carrying an attribute_ids column, paired with the
// dictionary scope its entries must have. Listed explicitly rather than
// discovered from the catalog: a new owner table should force someone to think
// about which scope it stores, not be silently covered by a loop.
var ownerArrays = []struct{ table, scope string }{
	{"spans", ingest.ScopeSpan},
	{"events", ingest.ScopeEvent},
	{"links", ingest.ScopeLink},
	{"logs", ingest.ScopeLog},
	{"datapoints", ingest.ScopeDatapoint},
	{"exemplars", ingest.ScopeExemplar},
	{"resources", ingest.ScopeResource},
	{"scopes", ingest.ScopeScope},
}

// assertNoDanglingRefs checks that every id in every owner array resolves to a
// dictionary row, and that it carries the scope that owner implies.
//
// The scope half matters as much as the existence half: an id implies its scope
// (scope is part of the content hash), which is what lets attribute discovery
// answer from the dictionary alone. If a span's array ever held a
// resource-scoped id, discovery would report the wrong attributeScope and the
// search-field dropdowns would quietly misclassify.
func assertNoDanglingRefs(t *testing.T, s *Store, when string) {
	t.Helper()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, o := range ownerArrays {
			var dangling int
			err := db.QueryRow(fmt.Sprintf(`
				select count(*) from (select unnest(attribute_ids) as id from %s) r
				where not exists (select 1 from attributes a where a.id = r.id)`, o.table)).Scan(&dangling)
			require.NoError(t, err)
			assert.Zero(t, dangling, "%s: %s references attribute ids that do not exist", when, o.table)

			var wrongScope int
			err = db.QueryRow(fmt.Sprintf(`
				select count(*) from (select unnest(attribute_ids) as id from %s) r
				join attributes a on a.id = r.id
				where a.scope <> ?`, o.table), o.scope).Scan(&wrongScope)
			require.NoError(t, err)
			assert.Zero(t, wrongScope, "%s: %s holds ids whose scope is not %q", when, o.table, o.scope)
		}
		return nil
	}))
}

func countIn(t *testing.T, s *Store, query string) int {
	t.Helper()
	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(query).Scan(&n)
	}))
	return n
}

// sharedResource is the same resource for all three signals, so the store ends
// up with one resources row referenced from spans, logs and metric_ingests --
// the cross-signal sharing the dictionary exists to make possible.
func sharedResource(res pcommon.Resource) {
	res.Attributes().PutStr("service.name", "checkout")
	res.Attributes().PutStr("host.name", "pod-a")
	res.Attributes().PutInt("process.pid", 4242)
}

func integrityTraces(seed byte) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	sharedResource(rs.Resource())
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("otelhttp")
	ss.Scope().SetVersion("1.2.0")
	ss.Scope().Attributes().PutStr("scope.tag", "primary")

	sp := ss.Spans().AppendEmpty()
	sp.SetTraceID([16]byte{seed, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	sp.SetSpanID([8]byte{seed, 2, 3, 4, 5, 6, 7, 8})
	sp.SetName("GET /checkout")
	sp.Attributes().PutStr("http.method", "GET")
	sp.Attributes().PutInt("http.status_code", 200)

	ev := sp.Events().AppendEmpty()
	ev.SetName("exception")
	ev.Attributes().PutStr("exception.type", "TimeoutError")

	lk := sp.Links().AppendEmpty()
	lk.SetTraceID([16]byte{99, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	lk.SetSpanID([8]byte{99, 2, 3, 4, 5, 6, 7, 8})
	lk.Attributes().PutStr("link.kind", "follows-from")
	return td
}

func integrityLogs(seed byte) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sharedResource(rl.Resource())
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("otelhttp")
	sl.Scope().SetVersion("1.2.0")
	sl.Scope().Attributes().PutStr("scope.tag", "primary")

	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + int64(seed)))
	lr.Body().SetStr("checkout failed")
	lr.SetSeverityText("ERROR")
	lr.Attributes().PutStr("log.origin", "handler")
	return ld
}

func integrityMetrics(seed byte) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sharedResource(rm.Resource())
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("otelhttp")
	sm.Scope().SetVersion("1.2.0")
	sm.Scope().Attributes().PutStr("scope.tag", "primary")

	m := sm.Metrics().AppendEmpty()
	m.SetName("http.server.duration")
	m.SetUnit("ms")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + int64(seed)))
	dp.SetDoubleValue(12.5)
	dp.Attributes().PutStr("http.route", "/checkout")

	ex := dp.Exemplars().AppendEmpty()
	ex.SetDoubleValue(12.5)
	ex.FilteredAttributes().PutStr("sampled.reason", "slow")
	return md
}

func ingestAll(t *testing.T, s *Store, seed byte) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, integrityTraces(seed), s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, integrityLogs(seed), s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, integrityMetrics(seed), s.FlushedIDs())
	}))
}

func sweep(t *testing.T, s *Store) {
	t.Helper()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(context.Background(), db, s.FlushedIDs())
	}))
}

// The headline case: clear a signal, sweep away what it orphaned, then ingest
// the same content again. Everything the second ingest references must exist.
//
// This is the guard for any future scheme that skips re-writing dictionary rows
// it believes are already present -- a per-store flush cache being the obvious
// one. Such a cache would remember the ids written before the Clear, skip them
// on re-ingest, and leave every new owner array pointing at rows the sweep had
// already deleted. No error, no failed constraint; the attributes would simply
// stop appearing. This test fails loudly instead.
func TestDictionaryIntegrityAcrossClearAndReingest(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)
	assertNoDanglingRefs(t, s, "after first ingest")

	// One resource and one scope, shared by all three signals.
	assert.Equal(t, 1, countIn(t, s, `select count(*) from resources`),
		"all three signals describe the same resource, so it must dedupe to one row")
	assert.Equal(t, 1, countIn(t, s, `select count(*) from scopes`))
	assert.Equal(t, 1, countIn(t, s,
		`select count(*) from resources r
		 where exists (select 1 from spans s where s.resource_id = r.id)
		   and exists (select 1 from logs l where l.resource_id = r.id)
		   and exists (select 1 from metric_ingests mi where mi.resource_id = r.id)`),
		"the one resource row must be referenced from spans, logs and metrics alike")

	before := countIn(t, s, `select count(*) from attributes`)

	// Clear traces. Logs and metrics still hold the shared resource and scope,
	// so those survive; only the span/event/link attributes become orphans.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.Clear(ctx, db)
	}))
	sweep(t, s)
	assertNoDanglingRefs(t, s, "after clearing traces")

	assert.Equal(t, 1, countIn(t, s, `select count(*) from resources`),
		"the resource is still referenced by logs and metrics, so the sweep must keep it")
	assert.Zero(t, countIn(t, s,
		fmt.Sprintf(`select count(*) from attributes where scope in ('%s','%s','%s')`,
			ingest.ScopeSpan, ingest.ScopeEvent, ingest.ScopeLink)),
		"span, event and link attributes have no owner left and must be swept")
	assert.Less(t, countIn(t, s, `select count(*) from attributes`), before,
		"the sweep must actually reclaim something, or this test proves nothing")

	// Re-ingest the identical content. Every id its arrays reference must be
	// present again, including the ones the sweep just deleted.
	ingestAll(t, s, 2)
	assertNoDanglingRefs(t, s, "after re-ingest following a clear")

	assert.Positive(t, countIn(t, s,
		fmt.Sprintf(`select count(*) from attributes where scope = '%s'`, ingest.ScopeSpan)),
		"span attributes deleted by the sweep must be written again on re-ingest")
}

// The sweep must not take rows that are still referenced. Clearing every signal
// in turn should leave the dictionary empty only once the last owner is gone.
func TestSweepKeepsSharedRowsUntilLastOwnerGoes(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return spans.Clear(ctx, db) }))
	sweep(t, s)
	assert.Equal(t, 1, countIn(t, s, `select count(*) from resources`), "logs and metrics still reference it")

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return logs.Clear(ctx, db) }))
	sweep(t, s)
	assert.Equal(t, 1, countIn(t, s, `select count(*) from resources`), "metrics still reference it")
	assertNoDanglingRefs(t, s, "after clearing traces and logs")

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return metrics.Clear(ctx, db) }))
	sweep(t, s)
	assert.Zero(t, countIn(t, s, `select count(*) from resources`), "the last owner is gone")
	assert.Zero(t, countIn(t, s, `select count(*) from scopes`))
	assert.Zero(t, countIn(t, s, `select count(*) from attributes`),
		"with no owners left the dictionary must be empty")
}

// Retention deletes rows too, and it is the other path that creates orphans.
// Its sweep must leave the same invariant intact.
func TestDictionaryIntegrityAfterRetention(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	for seed := byte(1); seed <= 20; seed++ {
		ingestAll(t, s, seed)
	}
	assertNoDanglingRefs(t, s, "after bulk ingest")

	// A cap of 1 byte is unreachable, so retention prunes its bounded number of
	// rounds and stops -- exercising every prune path and both sweep points.
	require.NoError(t, s.EnforceRetention(ctx, 1))
	assertNoDanglingRefs(t, s, "after retention")
}

// The cache's own contract, separate from the integrity it must not break.
//
// The middle assertion is the whole optimisation: a second batch of identical
// content adds no new ids, so its dictionary flush issues no SQL at all. That is
// what removes the ~2.3ms fixed per-batch cost, and it is why small batches went
// from 2.7x slower than the pre-dictionary schema to 2.1x faster.
//
// The final stretch pins the sweep's precise invalidation, not just that it
// invalidates something: clearing traces orphans the span/event/link
// attributes, but the resource, scope, and the log/metric attributes stay
// referenced, so those ids must survive in the cache. A sweep that fell back
// to forgetting everything would also make `after` smaller than `first` --
// only the `Positive` assertion on `after` tells the two apart.
func TestFlushedIDsPopulatesAndClears(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	assert.Zero(t, s.FlushedIDs().Len(), "nothing written yet")

	ingestAll(t, s, 1)
	first := s.FlushedIDs().Len()
	assert.Positive(t, first, "ingest must record what it wrote")

	ingestAll(t, s, 2)
	assert.Equal(t, first, s.FlushedIDs().Len(),
		"identical content adds no new ids -- this is the skip that makes it fast")

	// Clear traces only: span/event/link attributes lose their only owner and
	// become orphans, but the resource and scope are still referenced by logs
	// and metrics, and log/metric attributes are untouched.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.Clear(ctx, db)
	}))
	sweep(t, s)

	after := s.FlushedIDs().Len()
	assert.Less(t, after, first, "the sweep must forget the ids it actually deleted")
	assert.Positive(t, after,
		"ids the sweep did not delete -- the shared resource, scope, and log/metric attributes -- must stay marked")
}

// A store must never consult another store's set. Two stores are two databases;
// ids written to one say nothing about the other, and skipping on that basis
// would leave dangling references. This is the bug the first prototype hit.
func TestFlushedIDsAreNotSharedBetweenStores(t *testing.T) {
	ctx := context.Background()
	a, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer a.Close()
	b, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer b.Close()

	assert.NotSame(t, a.FlushedIDs(), b.FlushedIDs(), "each store owns its own set")

	ingestAll(t, a, 1)
	assert.Zero(t, b.FlushedIDs().Len(), "writing to one store must not mark the other")

	// b has seen nothing, so it writes everything and stays self-consistent.
	ingestAll(t, b, 1)
	assertNoDanglingRefs(t, b, "second store ingesting the same content")
}
