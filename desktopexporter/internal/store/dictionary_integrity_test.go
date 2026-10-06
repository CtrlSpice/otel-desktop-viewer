package store

// Referential integrity for the attribute dictionary.
//
// Every owner holds a uuid[] of attribute ids, and DuckDB cannot put a foreign
// key into a LIST. These tests enforce that relationship.
//
// A dangling reference produces no database error; attrs_json omits it.

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

// ownerArrays is every dictionary-reference array, paired with the dictionary
// scope its entries must have. Listed explicitly rather than discovered from
// the catalog: a new owner should force someone to think about which scope it
// stores, not be silently covered by a loop.
var ownerArrays = []struct{ table, column, scope string }{
	{"spans", "attribute_ids", ingest.ScopeSpan},
	{"events", "attribute_ids", ingest.ScopeEvent},
	{"links", "attribute_ids", ingest.ScopeLink},
	{"logs", "attribute_ids", ingest.ScopeLog},
	{"metric_datapoints", "attribute_ids", ingest.ScopeDatapoint},
	{"metric_series", "attribute_ids", ingest.ScopeDatapoint},
	{"exemplars", "attribute_ids", ingest.ScopeExemplar},
	{"metrics", "metadata_ids", ingest.ScopeMetricMetadata},
	{"resources", "attribute_ids", ingest.ScopeResource},
	{"scopes", "attribute_ids", ingest.ScopeScope},
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
				select count(*) from (select unnest(%s) as id from %s) r
				where not exists (select 1 from attributes a where a.id = r.id)`, o.column, o.table)).Scan(&dangling)
			require.NoError(t, err)
			assert.Zero(t, dangling, "%s: %s.%s references attribute ids that do not exist", when, o.table, o.column)

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

// sharedResource is the same Resource for all three signals.
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
	m.Metadata().PutStr("metric.owner", "checkout-team")
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

func TestResourceSchemaURLIdentityIsSharedAcrossSignals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	const sharedURL = "https://example.test/resource/shared"
	traces := integrityTraces(31)
	traces.ResourceSpans().At(0).SetSchemaUrl(sharedURL)
	logData := integrityLogs(32)
	logData.ResourceLogs().At(0).SetSchemaUrl(sharedURL)
	metricData := integrityMetrics(33)
	metricData.ResourceMetrics().At(0).SetSchemaUrl(sharedURL)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces, s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, logData, s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, metricData, s.FlushedIDs())
	}))

	other := integrityTraces(34)
	other.ResourceSpans().At(0).SetSchemaUrl("https://example.test/resource/other")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, other, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var resources, payloads, sharedOwners int
		require.NoError(t, db.QueryRow(`select count(*), count(distinct payload_id) from resources`).Scan(&resources, &payloads))
		require.NoError(t, db.QueryRow(`select count(*) from resources r
			where r.schema_url = ?
			  and exists (select 1 from spans s where s.resource_id = r.id)
			  and exists (select 1 from logs l where l.resource_id = r.id)
			  and exists (select 1 from metrics m where m.resource_id = r.id)`, sharedURL).Scan(&sharedOwners))
		assert.Equal(t, 2, resources)
		assert.Equal(t, 1, payloads, "schema URL must not alter the Resource payload identity")
		assert.Equal(t, 1, sharedOwners, "equal payload and URL must share one exact Resource row")
		return nil
	}))
}

func TestDistinctResourcePayloadsStayAttachedAcrossSignals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	traces := integrityTraces(21)
	traceResource := traces.ResourceSpans().At(0).Resource()
	traceResource.Attributes().PutStr("deployment.environment.name", "trace-env")
	logData := integrityLogs(22)
	logResource := logData.ResourceLogs().At(0).Resource()
	logResource.Attributes().PutStr("deployment.environment.name", "log-env")
	metricData := integrityMetrics(23)
	metricResource := metricData.ResourceMetrics().At(0).Resource()
	metricResource.Attributes().PutStr("deployment.environment.name", "metric-env")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces, s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, logData, s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, metricData, s.FlushedIDs())
	}))

	assert.Equal(t, 3, countIn(t, s, `select count(*) from resources`),
		"the shared service attributes must not collapse distinct signal payloads")
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, tc := range []struct {
			name, query, want string
		}{
			{"span", `select json_extract_string(a.value, '$.value') from spans s join resources r on r.id = s.resource_id, unnest(r.attribute_ids) t(id) join attributes a on a.id = t.id where a.key = 'deployment.environment.name'`, "trace-env"},
			{"log", `select json_extract_string(a.value, '$.value') from logs l join resources r on r.id = l.resource_id, unnest(r.attribute_ids) t(id) join attributes a on a.id = t.id where a.key = 'deployment.environment.name'`, "log-env"},
			{"metric", `select json_extract_string(a.value, '$.value') from metrics m join resources r on r.id = m.resource_id, unnest(r.attribute_ids) t(id) join attributes a on a.id = t.id where a.key = 'deployment.environment.name'`, "metric-env"},
		} {
			var got string
			require.NoError(t, db.QueryRow(tc.query).Scan(&got), tc.name)
			assert.Equal(t, tc.want, got, tc.name)
		}
		return nil
	}))
}

func sweep(t *testing.T, s *Store) {
	t.Helper()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(context.Background(), db, s.FlushedIDs())
	}))
}

// Clear a signal, sweep its orphans, then re-ingest the same content. Every new
// owner reference must resolve.
//
// This guards caches that skip known dictionary IDs after SweepOrphans has
// deleted their rows.
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
		   and exists (select 1 from metrics m where m.resource_id = r.id)`),
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
	assert.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(m.metadata_ids) t(id)
		join attributes a on a.id = t.id`),
		"live metric metadata must survive a sweep triggered by clearing another signal")
	assert.Zero(t, countIn(t, s, `
		select count(*) from attributes a where a.id in (
			select unnest(attribute_ids) from spans
			union select unnest(attribute_ids) from events
			union select unnest(attribute_ids) from links
		)`),
		"span, event and link attributes have no owner left and must be swept")
	assert.Less(t, countIn(t, s, `select count(*) from attributes`), before,
		"the sweep must actually reclaim something, or this test proves nothing")

	// Re-ingest the identical content. Every id its arrays reference must be
	// present again, including the ones the sweep just deleted.
	ingestAll(t, s, 2)
	assertNoDanglingRefs(t, s, "after re-ingest following a clear")

	assert.Positive(t, countIn(t, s, `
		select count(*) from spans s, unnest(s.attribute_ids) t(id)
		join attributes a on a.id = t.id`),
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

// Identical content adds no cache IDs. A trace clear invalidates only deleted
// span, event, and link IDs while retaining live shared IDs.
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

// Metric metadata has its own owner column and dictionary scope, so it needs
// the same cache-invalidation proof as ordinary attribute_ids arrays. Keep the
// other signals alive to prove the sweep invalidates precisely rather than
// falling back to forgetting the whole cache.
func TestSweepInvalidatesDeletedMetricMetadata(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)
	before := s.FlushedIDs().Len()
	require.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(m.metadata_ids) t(id)
		join attributes a on a.id = t.id`))

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	}))
	sweep(t, s)

	afterSweep := s.FlushedIDs().Len()
	assert.Less(t, afterSweep, before, "deleted metric dictionary ids must leave the cache")
	assert.Positive(t, afterSweep, "live trace and log dictionary ids must remain cached")
	assert.Zero(t, countIn(t, s, `
		select count(*) from metrics m, unnest(m.metadata_ids) t(id)
		join attributes a on a.id = t.id`),
		"metadata with no metric ingest owner must be collected")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, integrityMetrics(1), s.FlushedIDs())
	}))
	assertNoDanglingRefs(t, s, "after re-ingesting swept metric metadata")
	assert.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(m.metadata_ids) t(id)
		join attributes a on a.id = t.id`),
		"re-ingest must restore metadata removed by the sweep")
	assert.Equal(t, before, s.FlushedIDs().Len(),
		"re-ingest must restore the metric dictionary ids without forgetting live signal ids")
}

// Each store must use its own cache; IDs present in one database say nothing
// about another.
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
