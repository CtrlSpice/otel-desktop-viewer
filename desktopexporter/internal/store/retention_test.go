package store

import (
	"context"
	"go.uber.org/zap"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixed owner ids the seed helpers share. resource_id / scope_id are NOT NULL
// on spans and logs, so every fixture needs a real row to point at.
const (
	seedResourceID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	seedScopeID    = "ffffffff-ffff-ffff-ffff-ffffffffffff"
)

// seedOwners inserts the shared resource and scope rows, idempotently.
func seedOwners(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.db.Exec(`
		insert into resources (id, attribute_ids) values (?::uuid, []::uuid[])
		on conflict do nothing`, seedResourceID)
	require.NoError(t, err)
	_, err = s.db.Exec(`
		insert into scopes (id, name, version, attribute_ids)
		values (?::uuid, 'seed', 'v1', []::uuid[]) on conflict do nothing`, seedScopeID)
	require.NoError(t, err)
}

// seedSpans inserts n spans with start_time = i * 1ms (i in [0, n)), each
// referencing one fat attribute of its own so pruning visibly moves the size
// measurement.
//
// The attribute has to be per-span now. Under the old owner-keyed schema a
// single fat row per span was written n times; in the dictionary the same
// content collapses to one row, so a shared attribute would pad the store by
// 500 bytes total and the size assertions would measure nothing. Making each
// distinct also means the sweep has real garbage to collect after pruning,
// which is what the orphan assertions below check.
func seedSpans(t *testing.T, s *Store, n int) {
	t.Helper()
	seedOwners(t, s)
	_, err := s.db.Exec(`
		insert into attributes (id, key, value, type, scope)
		select attr_id('pad', repeat('x', 500) || range, 'string', 'span'),
		       'pad', repeat('x', 500) || range, 'string', 'span'
		from range(?) on conflict do nothing`, n)
	require.NoError(t, err)
	_, err = s.db.Exec(`
		insert into spans (trace_id, span_id, name, start_time, end_time,
		                   resource_id, scope_id, attribute_ids)
		select uuid(), uuid(), 'span-' || range, range * 1000000, range * 1000000 + 500,
		       ?::uuid, ?::uuid,
		       [attr_id('pad', repeat('x', 500) || range, 'string', 'span')]
		from range(?)`, seedResourceID, seedScopeID, n)
	require.NoError(t, err)
}

// seedLogs inserts n logs. Odd rows get timestamp = 0 to exercise the
// observed_timestamp fallback in the prune cutoff.
func seedLogs(t *testing.T, s *Store, n int) {
	t.Helper()
	seedOwners(t, s)
	_, err := s.db.Exec(`
		insert into logs (id, timestamp, observed_timestamp, body,
		                  resource_id, scope_id, attribute_ids)
		select uuid(),
			case when range % 2 = 0 then range * 1000000 else 0 end,
			range * 1000000,
			repeat('y', 200),
			?::uuid, ?::uuid, []::uuid[]
		from range(?)`, seedResourceID, seedScopeID, n)
	require.NoError(t, err)
}

// seedDatapoints inserts n datapoints for the given stream/ingest pair with
// timestamp = startTime + i * 1ms.
func seedDatapoints(t *testing.T, s *Store, streamID, ingestID string, n int, startTime int64) {
	t.Helper()
	_, err := s.db.Exec(`insert into metric_streams (id, name, metric_type) values (?, 'metric-' || ?, 'Gauge') on conflict do nothing`, streamID, streamID)
	require.NoError(t, err)
	seedOwners(t, s)
	_, err = s.db.Exec(`insert into metric_ingests (id, stream_id, resource_id, scope_id) values (?, ?, ?::uuid, ?::uuid)`,
		ingestID, streamID, seedResourceID, seedScopeID)
	require.NoError(t, err)
	// datapoints.series_id is a NOT NULL foreign key, so the series has to
	// exist before its points. One series per stream is enough here -- these
	// tests are about pruning by time, not about series identity.
	_, err = s.db.Exec(`
		insert into metric_series (id, stream_id, resource_id, attribute_ids)
		values (?::uuid, ?::uuid, ?::uuid, []::uuid[]) on conflict do nothing`,
		streamID, streamID, seedResourceID)
	require.NoError(t, err)
	_, err = s.db.Exec(`
		insert into datapoints (id, stream_id, series_id, metric_ingest_id, timestamp, double_value, value_type, attribute_ids)
		select uuid(), ?::uuid, ?::uuid, ?::uuid, ? + range * 1000000, range, 'double', []::uuid[]
		from range(?)`, streamID, streamID, ingestID, startTime, n)
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
	seedDatapoints(t, s, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", n, 0)

	// A cap of 1 byte is unreachable: enforcement should prune its bounded
	// number of rounds and stop, not loop forever.
	require.NoError(t, s.EnforceRetention(ctx, 1))

	for _, table := range []string{"spans", "logs", "datapoints"} {
		remaining := count(t, s, table)
		assert.Less(t, remaining, int64(n), "%s should have been pruned", table)
		assert.Positive(t, remaining, "%s should not have been emptied", table)
	}

	// The survivors must be the newest rows.
	var minStart int64
	require.NoError(t, s.db.QueryRow(`select min(start_time) from spans`).Scan(&minStart))
	assert.Positive(t, minStart, "the oldest spans should be gone")

	// The dictionary invariant, in both directions.
	//
	// Nothing enforces this: DuckDB cannot put a foreign key into a LIST, so
	// the relationship that used to be FK-checked is now ingest's and the
	// sweep's responsibility. Retention runs SweepOrphans at the end of every
	// round, so by the time enforcement returns there must be no attribute row
	// that nothing points at...
	var orphans int64
	require.NoError(t, s.db.QueryRow(`
		select count(*) from attributes a
		where not exists (
			select 1 from spans sp, unnest(sp.attribute_ids) t(aid) where t.aid = a.id
		)`).Scan(&orphans))
	assert.Zero(t, orphans, "retention must sweep attributes orphaned by pruning")

	// ...and, the direction that would be silent corruption rather than mere
	// garbage, no surviving span referencing an attribute that is gone.
	var dangling int64
	require.NoError(t, s.db.QueryRow(`
		select count(*) from (select unnest(attribute_ids) as id from spans) r
		where not exists (select 1 from attributes a where a.id = r.id)`).Scan(&dangling))
	assert.Zero(t, dangling, "pruning must never leave a span pointing at a missing attribute")
}

func TestEnforceRetentionSweepsOrphanedMetricIdentity(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	const oldStream = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const oldIngest = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	const liveStream = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	const liveIngest = "dddddddd-dddd-dddd-dddd-dddddddddddd"

	// One stream whose datapoints are all ancient, one with recent data far
	// enough ahead that pruning rounds never reach it.
	seedDatapoints(t, s, oldStream, oldIngest, 1000, 0)
	seedDatapoints(t, s, liveStream, liveIngest, 9000, 1_000_000_000_000)

	require.NoError(t, s.EnforceRetention(ctx, 1))

	var oldStreams, oldIngests, liveStreams int64
	require.NoError(t, s.db.QueryRow(`select count(*) from metric_streams where id = ?::uuid`, oldStream).Scan(&oldStreams))
	require.NoError(t, s.db.QueryRow(`select count(*) from metric_ingests where id = ?::uuid`, oldIngest).Scan(&oldIngests))
	require.NoError(t, s.db.QueryRow(`select count(*) from metric_streams where id = ?::uuid`, liveStream).Scan(&liveStreams))

	assert.Zero(t, oldStreams, "fully-pruned stream should be swept")
	assert.Zero(t, oldIngests, "ingest with no remaining datapoints should be swept")
	assert.Equal(t, int64(1), liveStreams, "stream with surviving datapoints must remain")
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

// TestSweepIfOverCapCollectsGarbage pins the guarantee sweepIfOverCap exists
// for: orphaned dictionary rows count toward the size the cap is compared
// against, so when the store is over, the orphans are collected *before* any
// prune decision is taken. Otherwise retention deletes real telemetry to make
// room for rows nothing references -- the garbage survives and the data does
// not.
//
// Asserting on the resulting size would not catch a regression here, because
// pruning also shrinks the store. Asserting that the live spans survived does.
func TestSweepIfOverCapCollectsGarbage(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	const total, keep = 4000, 200
	seedSpans(t, s, total)
	_, err = s.db.Exec(`delete from spans where start_time >= ?`, int64(keep)*1000000)
	require.NoError(t, err)
	require.Equal(t, int64(total), count(t, s, "attributes"),
		"the orphans must still be present, or this test proves nothing")

	size, err := s.SizeBytes(ctx)
	require.NoError(t, err)

	_, err = s.sweepIfOverCap(ctx, size-1)
	require.NoError(t, err)

	assert.Equal(t, int64(keep), count(t, s, "attributes"),
		"orphaned dictionary rows should have been collected before any prune")
	assert.Equal(t, int64(keep), count(t, s, "spans"),
		"sweeping must not touch live telemetry")
}

// TestSweepIfOverCapSkipsSweepUnderCap is the other half, and the behaviour
// change: a store comfortably inside its cap does no work at all. It used to
// sweep unconditionally on every retention tick, which also dropped the
// dictionary cache every 30 seconds for no reason.
func TestSweepIfOverCapSkipsSweepUnderCap(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	const total, keep = 1000, 100
	seedSpans(t, s, total)
	_, err = s.db.Exec(`delete from spans where start_time >= ?`, int64(keep)*1000000)
	require.NoError(t, err)

	fits, err := s.sweepIfOverCap(ctx, 1<<40 /* 1 TB */)
	require.NoError(t, err)
	assert.True(t, fits, "a store far under its cap must report that it fits")
	assert.Equal(t, int64(total), count(t, s, "attributes"),
		"under the cap there is nothing to protect against, so the sweep must not run")
}
