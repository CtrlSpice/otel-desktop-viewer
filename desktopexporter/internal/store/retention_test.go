package store

import (
	"context"
	"go.uber.org/zap"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixed identities for the row-owned Resource and Scope fixture payloads.
const (
	seedResourceID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	seedPayloadID  = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	seedScopeID    = "ffffffff-ffff-ffff-ffff-ffffffffffff"
)

// seedOwners defines constructors for row-owned fixture payloads.
func seedOwners(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.db.Exec(`create or replace macro seed_resource() as (
		{'id': '` + seedResourceID + `'::uuid, 'payload_id': '` + seedPayloadID + `'::uuid,
		 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0}::otel_resource);
		create or replace macro seed_scope() as (
		{'id': '` + seedScopeID + `'::uuid, 'name': 'seed', 'version': 'v1', 'schema_url': '',
		 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0}::otel_scope)`)
	require.NoError(t, err)
}

func TestPruneCutoffPreservesHighBitTimestamp(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	seedOwners(t, s)

	max := ^uint64(0)
	_, err = s.db.Exec(`insert into spans
		(trace_id, span_id, name, start_time, end_time, resource, scope, attributes)
		values (uuid(), 1::ubigint, 'max', (select unnest(?::ubigint[])),
			(select unnest(?::ubigint[])), seed_resource(), seed_scope(), '{}'::json::variant)`,
		[]uint64{max}, []uint64{max})
	require.NoError(t, err)

	cutoff, ok, err := s.pruneCutoff(ctx, s.db,
		`select quantile_disc(start_time, ?) from spans`)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, max, cutoff)
}

func TestPruneOldestSignalsMakeProgressWithSmallTables(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	seedSpans(t, s, 2)
	seedLogs(t, s, 2)
	const metricID = "11111111-1111-1111-1111-111111111111"
	seedDatapoints(t, s, metricID, 2, 0)

	require.NoError(t, s.pruneOldestSpans(ctx, s.db))
	require.NoError(t, s.pruneOldestLogs(ctx, s.db))
	require.NoError(t, s.pruneOldestDatapoints(ctx, s.db))

	for _, table := range []string{"spans", "logs", "metric_datapoints"} {
		var count int
		require.NoError(t, s.db.QueryRow(`select count(*) from `+table).Scan(&count))
		require.Equal(t, 1, count, table)
	}
}

// seedSpans inserts n spans with start_time = i * 1ms (i in [0, n)), each
// owning one large attribute so pruning visibly moves the size
// measurement.
//
// Each span gets a distinct attribute payload.
func seedSpans(t *testing.T, s *Store, n int) {
	t.Helper()
	seedOwners(t, s)
	_, err := s.db.Exec(`
		insert into spans (trace_id, span_id, name, start_time, end_time,
		                   resource, scope, attributes)
		select uuid(), range::ubigint, 'span-' || range, range * 1000000, range * 1000000 + 500,
		       seed_resource(), seed_scope(), {'pad': repeat('x', 500) || range}::variant
		from range(?)`, n)
	require.NoError(t, err)
}

// seedLogs inserts n logs. Odd rows get timestamp = 0 to exercise the
// observed_timestamp fallback in the prune cutoff.
func seedLogs(t *testing.T, s *Store, n int) {
	t.Helper()
	seedOwners(t, s)
	_, err := s.db.Exec(`
		insert into logs (id, timestamp, observed_timestamp, body,
		                  resource, scope, attributes)
		select uuid(),
			case when range % 2 = 0 then range * 1000000 else 0 end,
			range * 1000000,
			repeat('y', 200)::variant,
			seed_resource(), seed_scope(), '{}'::json::variant
		from range(?)`, n)
	require.NoError(t, err)
}

// seedDatapoints inserts n datapoints with timestamp = startTime + i * 1ms.
func seedDatapoints(t *testing.T, s *Store, metricID string, n int, startTime int64) {
	t.Helper()
	seedOwners(t, s)
	_, err := s.db.Exec(`insert into metric_records
		(record_id, id, resource_id, resource_payload_id, scope_id, name, metric_type,
		 resource, scope, datapoints, point_time_min, point_time_max)
		select uuid(), $1::uuid, $2::uuid, $3::uuid, $4::uuid, 'metric-' || $1, 'Gauge', seed_resource(), seed_scope(),
		 list(struct_pack(id:=uuid(), timestamp:=$5::ubigint+range*1000000,
		 double_value:=range::double, value_type:='Double', attribute_key:='', attributes:='{}'::json::variant)),
		 $5::ubigint, $5::ubigint+($6::ubigint-1)*1000000
		 from range($6)`, metricID, seedResourceID, seedPayloadID, seedScopeID, startTime, n)
	require.NoError(t, err)
}

func count(t *testing.T, s *Store, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.db.QueryRow(`select count(*) from `+table).Scan(&n))
	return n
}

func TestSizeBytesInMemory(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	empty, err := s.SizeBytes(ctx)
	require.NoError(t, err)

	seedSpans(t, s, 5000)

	seeded, err := s.SizeBytes(ctx)
	require.NoError(t, err)
	assert.Greater(t, seeded, empty, "size should grow with data")
}

func TestSizeBytesOnDisk(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, filepath.Join(t.TempDir(), "retention_test.db"), zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 5000)
	_, err = s.db.Exec(`checkpoint`)
	require.NoError(t, err)

	size, err := s.SizeBytes(ctx)
	require.NoError(t, err)
	assert.Positive(t, size, "database file should have a measurable size")
}

func TestEnforceRetentionPrunesOldest(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	const n = 10000
	seedSpans(t, s, n)
	seedLogs(t, s, n)
	seedDatapoints(t, s, "11111111-1111-1111-1111-111111111111", n, 0)

	// A cap of 1 byte is unreachable: enforcement should prune its bounded
	// number of rounds and stop, not loop forever.
	require.NoError(t, s.EnforceRetention(ctx, 1))

	for _, table := range []string{"spans", "logs", "metric_datapoints"} {
		remaining := count(t, s, table)
		assert.Less(t, remaining, int64(n), "%s should have been pruned", table)
		assert.Positive(t, remaining, "%s should not have been emptied", table)
	}

	// The survivors must be the newest rows.
	var minStart int64
	require.NoError(t, s.db.QueryRow(`select min(start_time) from spans`).Scan(&minStart))
	assert.Positive(t, minStart, "the oldest spans should be gone")

	// Surviving spans retain the complete payload corresponding to their time.
	var dangling int64
	require.NoError(t, s.db.QueryRow(`
		select count(*) from spans where attributes.pad::varchar is distinct from
		(repeat('x', 500) || (start_time // 1000000))`).Scan(&dangling))
	assert.Zero(t, dangling, "pruning must retain the matching attributes")
}

func TestEnforceRetentionRemovesFullyPrunedMetricRecords(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	const oldMetric = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const liveMetric = "cccccccc-cccc-cccc-cccc-cccccccccccc"

	// One Metric has only ancient datapoints; one has recent data far
	// enough ahead that pruning rounds never reach it.
	seedDatapoints(t, s, oldMetric, 1000, 0)
	seedDatapoints(t, s, liveMetric, 9000, 1_000_000_000_000)

	require.NoError(t, s.EnforceRetention(ctx, 1))

	var oldMetrics, oldSeries, liveMetrics int64
	require.NoError(t, s.db.QueryRow(`select count(*) from metrics where id = ?::uuid`, oldMetric).Scan(&oldMetrics))
	require.NoError(t, s.db.QueryRow(`select count(*) from metric_series where metric_id = ?::uuid`, oldMetric).Scan(&oldSeries))
	require.NoError(t, s.db.QueryRow(`select count(*) from metrics where id = ?::uuid`, liveMetric).Scan(&liveMetrics))

	assert.Zero(t, oldMetrics, "fully pruned received records are removed")
	assert.Zero(t, oldSeries, "series with no remaining points are removed")
	assert.Equal(t, int64(1), liveMetrics, "Metric with surviving datapoints must remain")

	var activeMetrics, activeSeries int64
	require.NoError(t, s.db.QueryRow(`select count(distinct metric_id) from metric_datapoints`).Scan(&activeMetrics))
	require.NoError(t, s.db.QueryRow(`select count(distinct series_id) from metric_datapoints`).Scan(&activeSeries))
	assert.Equal(t, int64(1), activeMetrics, "retained identity-only Metrics must not count as active")
	assert.Equal(t, int64(1), activeSeries, "retained identity-only series must not count as active")
}

func TestEnforceRetentionDisabled(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 1000)

	require.NoError(t, s.EnforceRetention(ctx, 0))
	assert.Equal(t, int64(1000), count(t, s, "spans"), "cap of 0 must disable pruning")
}

func TestEnforceRetentionUnderCap(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 1000)

	require.NoError(t, s.EnforceRetention(ctx, 1<<40 /* 1 TB */))
	assert.Equal(t, int64(1000), count(t, s, "spans"), "store under the cap must not be pruned")
}

// Pruning an old span must preserve children of a newer span in another trace
// that shares its span ID.
func TestEnforceRetentionPrunesChildrenByTraceAndSpan(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	seedOwners(t, s)
	// Bulk rows so retention has something worth pruning; their timestamps
	// (range * 1ms) are ancient next to the two probe traces below.
	seedSpans(t, s, 5000)

	// One span id, two traces: old will fall to the prune, new must survive
	// whole. now is far above every seeded timestamp, so the cutoff -- a
	// quantile over start_time -- lands between the bulk and the new probe.
	const sharedSpanID = "15987178197214944733"
	const oldTrace = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const newTrace = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	now := int64(1) << 60

	for _, tr := range []struct {
		trace string
		start int64
	}{
		{oldTrace, 1}, // older than every bulk row
		{newTrace, now},
	} {
		_, err = s.db.Exec(`
			insert into spans (trace_id, span_id, name, start_time, end_time,
			                   resource, scope, attributes)
			values (?::uuid, ?::ubigint, 'probe', ?, ?, seed_resource(), seed_scope(), '{}'::json::variant)`,
			tr.trace, sharedSpanID, tr.start, tr.start+1)
		require.NoError(t, err)
		_, err = s.db.Exec(`update spans set
			events=[struct_pack(id:=uuid(), name:='probe-event', timestamp:=$3::ubigint, attributes:='{}'::json::variant)],
			links=[struct_pack(id:=uuid(), attributes:='{}'::json::variant)]
			where trace_id=$1::uuid and span_id=$2::ubigint`, tr.trace, sharedSpanID, tr.start)
		require.NoError(t, err)
	}

	require.NoError(t, s.EnforceRetention(ctx, 1))

	// The old probe span must be gone and the new one intact.
	var oldSpans, newSpans, newEvents, newLinks int64
	require.NoError(t, s.db.QueryRow(
		`select count(*) from spans where trace_id = ?::uuid`, oldTrace).Scan(&oldSpans))
	require.NoError(t, s.db.QueryRow(
		`select count(*) from spans where trace_id = ?::uuid`, newTrace).Scan(&newSpans))
	require.NoError(t, s.db.QueryRow(
		`select count(*) from events where trace_id = ?::uuid`, newTrace).Scan(&newEvents))
	require.NoError(t, s.db.QueryRow(
		`select count(*) from links where trace_id = ?::uuid`, newTrace).Scan(&newLinks))

	assert.Zero(t, oldSpans, "the old trace's span should have been pruned")
	require.EqualValues(t, 1, newSpans, "the new trace's span must survive the prune")
	assert.EqualValues(t, 1, newEvents,
		"the surviving span's events were deleted through a span-id-only match")
	assert.EqualValues(t, 1, newLinks,
		"the surviving span's links were deleted through a span-id-only match")

	// And no orphans in the other direction: every remaining child has its span.
	var orphanEvents int64
	require.NoError(t, s.db.QueryRow(`
		select count(*) from events e
		where not exists (select 1 from spans sp
			where sp.trace_id = e.trace_id and sp.span_id = e.span_id)`).Scan(&orphanEvents))
	assert.Zero(t, orphanEvents, "pruning left events whose span is gone")
}
