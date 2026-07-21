package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedSpans inserts n spans with start_time = i * 1ms (i in [0, n)), each with
// one fat attribute row so pruning visibly moves the size measurement.
func seedSpans(t *testing.T, s *Store, n int) {
	t.Helper()
	_, err := s.DB().Exec(`
		insert into spans (trace_id, span_id, name, start_time, end_time)
		select uuid(), uuid(), 'span-' || range, range * 1000000, range * 1000000 + 500
		from range(?)`, n)
	require.NoError(t, err)
	_, err = s.DB().Exec(`
		insert into attributes (span_id, scope, key, value, type)
		select span_id, 'span', 'pad', repeat('x', 500), 'string' from spans`)
	require.NoError(t, err)
}

// seedLogs inserts n logs. Odd rows get timestamp = 0 to exercise the
// observed_timestamp fallback in the prune cutoff.
func seedLogs(t *testing.T, s *Store, n int) {
	t.Helper()
	_, err := s.DB().Exec(`
		insert into logs (id, timestamp, observed_timestamp, body)
		select uuid(),
			case when range % 2 = 0 then range * 1000000 else 0 end,
			range * 1000000,
			repeat('y', 200)
		from range(?)`, n)
	require.NoError(t, err)
}

// seedDatapoints inserts n datapoints for the given stream/ingest pair with
// timestamp = startTime + i * 1ms.
func seedDatapoints(t *testing.T, s *Store, streamID, ingestID string, n int, startTime int64) {
	t.Helper()
	_, err := s.DB().Exec(`insert into metric_streams (id, name, metric_type) values (?, 'metric-' || ?, 'Gauge') on conflict do nothing`, streamID, streamID)
	require.NoError(t, err)
	_, err = s.DB().Exec(`insert into metric_ingests (id, stream_id) values (?, ?)`, ingestID, streamID)
	require.NoError(t, err)
	_, err = s.DB().Exec(`
		insert into datapoints (id, stream_id, metric_ingest_id, timestamp, double_value, value_type)
		select uuid(), ?::uuid, ?::uuid, ? + range * 1000000, range, 'double'
		from range(?)`, streamID, ingestID, startTime, n)
	require.NoError(t, err)
}

func count(t *testing.T, s *Store, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.DB().QueryRow(`select count(*) from `+table).Scan(&n))
	return n
}

func TestSizeBytesInMemory(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
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
	s, err := NewStore(ctx, filepath.Join(t.TempDir(), "retention_test.db"))
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 5000)
	_, err = s.DB().Exec(`checkpoint`)
	require.NoError(t, err)

	size, err := s.SizeBytes(ctx)
	require.NoError(t, err)
	assert.Positive(t, size, "database file should have a measurable size")
}

func TestEnforceRetentionPrunesOldest(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
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
	require.NoError(t, s.DB().QueryRow(`select min(start_time) from spans`).Scan(&minStart))
	assert.Positive(t, minStart, "the oldest spans should be gone")

	// No dangling attributes: every attribute's span must still exist.
	var orphans int64
	require.NoError(t, s.DB().QueryRow(`
		select count(*) from attributes a
		where a.span_id is not null
		and not exists (select 1 from spans sp where sp.span_id = a.span_id)`).Scan(&orphans))
	assert.Zero(t, orphans, "pruning must not leave orphaned attributes")
}

func TestEnforceRetentionSweepsOrphanedMetricIdentity(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
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
	require.NoError(t, s.DB().QueryRow(`select count(*) from metric_streams where id = ?::uuid`, oldStream).Scan(&oldStreams))
	require.NoError(t, s.DB().QueryRow(`select count(*) from metric_ingests where id = ?::uuid`, oldIngest).Scan(&oldIngests))
	require.NoError(t, s.DB().QueryRow(`select count(*) from metric_streams where id = ?::uuid`, liveStream).Scan(&liveStreams))

	assert.Zero(t, oldStreams, "fully-pruned stream should be swept")
	assert.Zero(t, oldIngests, "ingest with no remaining datapoints should be swept")
	assert.Equal(t, int64(1), liveStreams, "stream with surviving datapoints must remain")
}

func TestEnforceRetentionDisabled(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 1000)

	require.NoError(t, s.EnforceRetention(ctx, 0))
	assert.Equal(t, int64(1000), count(t, s, "spans"), "cap of 0 must disable pruning")
}

func TestEnforceRetentionUnderCap(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)
	defer s.Close()

	seedSpans(t, s, 1000)

	require.NoError(t, s.EnforceRetention(ctx, 1<<40 /* 1 TB */))
	assert.Equal(t, int64(1000), count(t, s, "spans"), "store under the cap must not be pruned")
}
