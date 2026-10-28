package store

// Owned payload integrity across ingestion, clear, retention, and reopen.

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

// List owners explicitly so new payload columns require coverage here.
var ownerColumns = []struct{ table, column string }{
	{"spans", "attributes"},
	{"events", "attributes"},
	{"links", "attributes"},
	{"logs", "attributes"},
	{"metric_datapoints", "attributes"},
	{"metric_series", "attributes"},
	{"exemplars", "attributes"},
	{"metrics", "metadata"},
	{"spans", "resource.attributes"},
	{"spans", "scope.attributes"},
	{"logs", "resource.attributes"},
	{"logs", "scope.attributes"},
	{"metrics", "resource.attributes"},
	{"metrics", "scope.attributes"},
	{"metric_datapoints", "resource.attributes"},
	{"metric_datapoints", "scope.attributes"},
}

func assertOwnedMaps(t *testing.T, s *Store, when string) {
	t.Helper()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, o := range ownerColumns {
			var dangling int
			err := db.QueryRow(fmt.Sprintf(`
				select count(*) from %s where %s is null or otel_value_kind(%s) <> 'map'`, o.table, o.column, o.column)).Scan(&dangling)
			require.NoError(t, err)
			assert.Zero(t, dangling, "%s: %s.%s must retain its attribute map", when, o.table, o.column)

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
		return spans.Ingest(ctx, conn, integrityTraces(seed))
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, integrityLogs(seed))
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
		return spans.Ingest(ctx, conn, traces)
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, logData)
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, metricData, s.FlushedIDs())
	}))

	other := integrityTraces(34)
	other.ResourceSpans().At(0).SetSchemaUrl("https://example.test/resource/other")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, other)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var resources, payloads, sharedOwners int
		require.NoError(t, db.QueryRow(`select count(distinct resource.id), count(distinct resource.payload_id) from spans`).Scan(&resources, &payloads))
		require.NoError(t, db.QueryRow(`select count(*) from spans s
			where s.resource.schema_url = ?
			  and exists (select 1 from logs l where l.resource.id = s.resource.id)
			  and exists (select 1 from metrics m where m.resource.id = s.resource.id)`, sharedURL).Scan(&sharedOwners))
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
		return spans.Ingest(ctx, conn, traces)
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, logData)
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, metricData, s.FlushedIDs())
	}))

	assert.Equal(t, 3, countIn(t, s, `select count(*) from (select resource.id from spans union select resource.id from logs union select resource.id from metrics)`),
		"the shared service attributes must not collapse distinct signal payloads")
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, tc := range []struct {
			name, query, want string
		}{
			{"span", `select resource.attributes['deployment.environment.name']::varchar from spans`, "trace-env"},
			{"log", `select resource.attributes['deployment.environment.name']::varchar from logs`, "log-env"},
			{"metric", `select resource.attributes['deployment.environment.name']::varchar from metrics`, "metric-env"},
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

// Clearing and re-ingesting one signal preserves every other signal's payloads.
func TestOwnershipAcrossClearAndReingest(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)
	assertOwnedMaps(t, s, "after first ingest")

	// One resource and one scope, shared by all three signals.
	assert.Equal(t, 1, countIn(t, s, `select count(distinct resource.id) from spans`),
		"identical Resource content must retain one grouping identity")
	assert.Equal(t, 1, countIn(t, s, `select count(distinct scope.id) from spans`))
	assert.Equal(t, 1, countIn(t, s,
		`select count(*) from spans s
		 where exists (select 1 from logs l where l.resource.id = s.resource.id)
		   and exists (select 1 from metrics m where m.resource.id = s.resource.id)`),
		"Resource grouping identity must agree across signals")

	// Clear traces. Logs and metrics retain their own Resource and Scope.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.Clear(ctx, db)
	}))
	sweep(t, s)
	assertOwnedMaps(t, s, "after clearing traces")

	assert.Equal(t, 1, countIn(t, s, `select count(distinct resource.id) from logs`),
		"logs retain their Resource after clearing spans")
	assert.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(variant_keys(m.metadata)) t(key)`),
		"live metric metadata must survive a sweep triggered by clearing another signal")
	assert.Zero(t, countIn(t, s, `
		select (select count(*) from spans) + (select count(*) from events) + (select count(*) from links)`),
		"span, event and link payloads are removed with their rows")

	// Re-ingest identical content and verify every owned map is restored.
	ingestAll(t, s, 2)
	assertOwnedMaps(t, s, "after re-ingest following a clear")

	assert.Positive(t, countIn(t, s, `
		select count(*) from spans s, unnest(variant_keys(s.attributes)) t(key)`),
		"span attributes removed by Clear must be written again on re-ingest")
}

// Removing one signal cannot remove the context owned by another signal.
func TestClearKeepsOtherSignalsOwnedContext(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return spans.Clear(ctx, db) }))
	sweep(t, s)
	assert.Equal(t, 1, countIn(t, s, `select count(distinct resource.id) from logs`), "logs retain their resource")

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return logs.Clear(ctx, db) }))
	sweep(t, s)
	assert.Equal(t, 1, countIn(t, s, `select count(distinct resource.id) from metrics`), "metrics retain their resource")
	assertOwnedMaps(t, s, "after clearing traces and logs")

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return metrics.Clear(ctx, db) }))
	sweep(t, s)
	assert.Zero(t, countIn(t, s, `select count(*) from metrics`))
	assert.Zero(t, countIn(t, s, `select count(*) from duckdb_tables() where table_name in ('attributes','resources','scopes')`))
}

// Retention deletes rows too, and it is the other path that creates orphans.
// Its sweep must leave the same invariant intact.
func TestOwnershipAfterRetention(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	for seed := byte(1); seed <= 20; seed++ {
		ingestAll(t, s, seed)
	}
	assertOwnedMaps(t, s, "after bulk ingest")

	// A cap of 1 byte is unreachable, so retention prunes its bounded number of
	// rounds and stops -- exercising every prune path and both sweep points.
	require.NoError(t, s.EnforceRetention(ctx, 1))
	assertOwnedMaps(t, s, "after retention")
}

func TestOwnedSignalsDoNotPopulateBoundsCache(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	assert.Zero(t, s.FlushedIDs().Len(), "nothing written yet")

	ingestAll(t, s, 1)
	first := s.FlushedIDs().Len()
	assert.Zero(t, first, "non-histogram signals need no shared payload cache")

	ingestAll(t, s, 2)
	assert.Equal(t, first, s.FlushedIDs().Len(),
		"owned payloads do not enter the bounds cache")

	// Clear traces while logs and metrics retain their own context.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.Clear(ctx, db)
	}))
	sweep(t, s)

	after := s.FlushedIDs().Len()
	assert.Zero(t, after, "clearing spans requires no payload cache invalidation")
}

// Metadata can be re-ingested after clearing metrics while other signals remain.
func TestMetricMetadataClearAndReingest(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)
	before := s.FlushedIDs().Len()
	require.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(variant_keys(m.metadata)) t(key)`))

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	}))
	sweep(t, s)

	afterSweep := s.FlushedIDs().Len()
	assert.Equal(t, before, afterSweep, "owned metadata requires no cache invalidation")
	assert.Zero(t, countIn(t, s, `
		select count(*) from metrics m, unnest(variant_keys(m.metadata)) t(key)`),
		"metadata with no metric ingest owner must be collected")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, integrityMetrics(1), s.FlushedIDs())
	}))
	assertOwnedMaps(t, s, "after re-ingesting metric metadata")
	assert.Equal(t, 1, countIn(t, s, `
		select count(*) from metrics m, unnest(variant_keys(m.metadata)) t(key)`),
		"re-ingest must restore metadata removed with its metric")
	assert.Equal(t, before, s.FlushedIDs().Len(),
		"owned metadata does not enter the bounds cache")
}

// Receiving identical data in one store cannot suppress payloads in another.
func TestOwnedPayloadsAreIndependentBetweenStores(t *testing.T) {
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
	assertOwnedMaps(t, b, "second store ingesting the same content")
}
