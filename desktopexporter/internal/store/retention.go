package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
)

var ErrRetentionInternal = errors.New("retention internal error")

const (
	// pruneFraction is the share of the oldest telemetry (by time percentile)
	// deleted per pruning round. Small enough that one round rarely erases
	// history someone is actively looking at; EnforceRetention loops when a
	// single round doesn't free enough space.
	pruneFraction = 0.10

	// maxPruneRounds bounds the prune-measure loop within one enforcement
	// pass. Deleted bytes don't map linearly to rows, so a round can
	// under-deliver; three compounding rounds (~27%) is plenty for one pass,
	// and the next pass picks up from there.
	maxPruneRounds = 3
)

// SizeBytes reports the current size of the telemetry store.
//
// In disk mode this is the size of the database file plus its write-ahead
// log, which is what actually occupies the user's disk. In in-memory mode
// it is the total tracked by DuckDB's duckdb_memory() table function, since
// there is no file to measure.
func (s *Store) SizeBytes(ctx context.Context) (int64, error) {
	var size int64
	err := s.WithDBRead(func(db *sql.DB) error {
		var err error
		size, err = s.sizeBytes(ctx, db)
		return err
	})
	return size, err
}

// SizeBytesWithDB reports store size using a *sql.DB the caller already
// obtained from WithDBRead or WithDBWrite. Use this instead of SizeBytes when
// already inside a WithDB* callback: calling SizeBytes there would acquire the
// read lock a second time, which deadlocks if a writer queues between the two
// acquisitions.
func (s *Store) SizeBytesWithDB(ctx context.Context, db *sql.DB) (int64, error) {
	return s.sizeBytes(ctx, db)
}

// sizeBytes is the unlocked implementation. Callers must already hold s.mu.
func (s *Store) sizeBytes(ctx context.Context, db *sql.DB) (int64, error) {
	if s.dbPath == "" {
		var size int64
		err := db.QueryRowContext(ctx,
			`select coalesce(sum(memory_usage_bytes), 0) from duckdb_memory()`,
		).Scan(&size)
		if err != nil {
			return 0, fmt.Errorf("SizeBytes: %w: %w", ErrRetentionInternal, err)
		}
		return size, nil
	}

	var total int64
	for _, path := range []string{s.dbPath, s.dbPath + ".wal"} {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			// The WAL is transient; it not existing is normal.
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("SizeBytes: %w: %w", ErrRetentionInternal, err)
		}
		total += info.Size()
	}
	return total, nil
}

// EnforceRetention prunes the oldest telemetry until the store fits within
// maxBytes. maxBytes <= 0 disables enforcement. Each round deletes the oldest
// pruneFraction of every signal (by time percentile) and checkpoints so
// DuckDB reclaims the space, re-measuring between rounds.
//
// The write lock is taken per round, not for the whole pass, so queries can
// interleave between rounds. A full pass runs up to three checkpoints; holding
// the lock across all of them would block every reader for the entire pass.
func (s *Store) EnforceRetention(ctx context.Context, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}

	// Orphaned dictionary rows count toward the size the cap is compared
	// against, so garbage left by a Clear or a delete-by-id must never be what
	// tips the store over and triggers pruning of real telemetry. That requires
	// sweeping before any prune -- but not before the measurement that decides
	// whether to prune at all.
	//
	// This used to sweep unconditionally, which meant a store sitting at 1% of
	// its cap still ran three anti-joins and dropped the dictionary cache every
	// 30 seconds, under the write lock, to protect against a prune that was
	// never going to happen.
	fits, err := s.sweepIfOverCap(ctx, maxBytes)
	if err != nil {
		return err
	}
	if fits {
		return nil
	}

	for round := 0; round < maxPruneRounds; round++ {
		fits, err := s.enforceRound(ctx, maxBytes)
		if err != nil {
			return err
		}
		if fits {
			return nil
		}
	}
	return nil
}

// sweepIfOverCap measures the store and, only if it exceeds maxBytes, collects
// orphans and measures again. It reports whether the store fits, in which case
// no pruning is needed.
//
// The re-measurement is what preserves the guarantee: if the excess was garbage
// all along, the sweep alone brings the store back under and this returns true,
// so no telemetry is pruned. Sweeping and re-measuring costs a checkpoint, but
// only on the path where something is actually over the cap.
func (s *Store) sweepIfOverCap(ctx context.Context, maxBytes int64) (bool, error) {
	fits := false
	err := s.WithDBWrite(func(db *sql.DB) error {
		size, err := s.sizeBytes(ctx, db)
		if err != nil {
			return err
		}
		if size <= maxBytes {
			fits = true
			return nil
		}

		if err := ingest.SweepOrphans(ctx, db, s.flushed); err != nil {
			return err
		}
		// Without a checkpoint the deletes do not move the measurement, so the
		// re-measure below would report the pre-sweep size and prune anyway.
		if _, err := db.ExecContext(ctx, `checkpoint`); err != nil {
			return fmt.Errorf("EnforceRetention: %w: %w", ErrRetentionInternal, err)
		}

		size, err = s.sizeBytes(ctx, db)
		if err != nil {
			return err
		}
		fits = size <= maxBytes
		return nil
	})
	return fits, err
}

// enforceRound runs one measure-prune-checkpoint round under the write lock.
// It reports whether the store already fits within maxBytes, in which case no
// pruning was done and the caller can stop.
func (s *Store) enforceRound(ctx context.Context, maxBytes int64) (bool, error) {
	fits := false
	err := s.WithDBWrite(func(db *sql.DB) error {
		size, err := s.sizeBytes(ctx, db)
		if err != nil {
			return err
		}
		if size <= maxBytes {
			fits = true
			return nil
		}

		if err := s.pruneOldestSpans(ctx, db); err != nil {
			return err
		}
		if err := s.pruneOldestLogs(ctx, db); err != nil {
			return err
		}
		if err := s.pruneOldestDatapoints(ctx, db); err != nil {
			return err
		}

		// The prunes are what create orphans, so sweep here rather than at the
		// top of the next round: the next round's measurement then reflects
		// live data instead of what this round just abandoned.
		if err := ingest.SweepOrphans(ctx, db, s.flushed); err != nil {
			return err
		}

		// Checkpoint flushes the WAL and lets DuckDB reuse the freed blocks;
		// without it the file/memory measurement would not move.
		if _, err := db.ExecContext(ctx, `checkpoint`); err != nil {
			return fmt.Errorf("EnforceRetention: %w: %w", ErrRetentionInternal, err)
		}
		return nil
	})
	return fits, err
}

// pruneCutoff returns the timestamp below which rows should be deleted,
// i.e. the pruneFraction percentile of the given time expression. Returns
// (0, false) when the table is empty.
func (s *Store) pruneCutoff(ctx context.Context, db *sql.DB, query string) (int64, bool, error) {
	var cutoff sql.NullInt64
	if err := db.QueryRowContext(ctx, query, pruneFraction).Scan(&cutoff); err != nil {
		return 0, false, fmt.Errorf("pruneCutoff: %w: %w", ErrRetentionInternal, err)
	}
	return cutoff.Int64, cutoff.Valid, nil
}

// pruneOldestSpans deletes the oldest fraction of spans along with their
// events and links. Leaves first, spans last.
//
// Attributes are not touched here: they are shared dictionary rows, and whether
// a given one is still referenced is not a question a span predicate can answer.
// enforceRound sweeps once after all three prunes.
func (s *Store) pruneOldestSpans(ctx context.Context, db *sql.DB) error {
	cutoff, ok, err := s.pruneCutoff(ctx, db,
		`select cast(quantile_cont(start_time, ?) as bigint) from spans`)
	if err != nil || !ok {
		return err
	}

	for _, q := range []string{
		`delete from links where span_id in (select span_id from spans where start_time < ?)`,
		`delete from events where span_id in (select span_id from spans where start_time < ?)`,
		`delete from spans where start_time < ?`,
	} {
		if _, err := db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestSpans: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}

// pruneOldestLogs deletes the oldest fraction of logs. Attributes are swept
// separately, for the reason given on pruneOldestSpans.
// Logs may arrive with timestamp = 0 (unset); observed_timestamp is the
// fallback, mirroring how GetStats computes lastReceived.
func (s *Store) pruneOldestLogs(ctx context.Context, db *sql.DB) error {
	const logTime = `coalesce(nullif(timestamp, 0), observed_timestamp)`

	cutoff, ok, err := s.pruneCutoff(ctx, db,
		`select cast(quantile_cont(`+logTime+`, ?) as bigint) from logs`)
	if err != nil || !ok {
		return err
	}

	for _, q := range []string{
		`delete from logs where ` + logTime + ` < ?`,
	} {
		if _, err := db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestLogs: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}

// pruneOldestDatapoints deletes the oldest fraction of datapoints with their
// exemplars, then sweeps metric_ingests and metric_streams
// rows that no longer own any datapoints. The identity sweep matters:
// metric_ingests grows by one row per OTLP batch, so leaving orphans behind
// would let the store creep back over the cap with rows pruning can't touch.
// A swept stream that is still live gets recreated by ingest's find-or-insert.
func (s *Store) pruneOldestDatapoints(ctx context.Context, db *sql.DB) error {
	cutoff, ok, err := s.pruneCutoff(ctx, db,
		`select cast(quantile_cont(timestamp, ?) as bigint) from datapoints`)
	if err != nil || !ok {
		return err
	}

	doomed := `(select id from datapoints where timestamp < ?)`
	for _, q := range []string{
		`delete from exemplars where datapoint_id in ` + doomed,
		`delete from datapoints where timestamp < ?`,
	} {
		if _, err := db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestDatapoints: %w: %w", ErrRetentionInternal, err)
		}
	}

	// Orphan sweep: ingest batches whose datapoints are all gone, then
	// streams whose ingest batches are all gone. Ordering follows the FK
	// chain (metric_ingests -> metric_streams).
	for _, q := range []string{
		`delete from metric_series ms
			where not exists (select 1 from datapoints d where d.series_id = ms.id)`,
		`delete from metric_ingests mi
			where not exists (select 1 from datapoints d where d.metric_ingest_id = mi.id)`,
		`delete from metric_streams ms
			where not exists (select 1 from metric_ingests mi where mi.stream_id = ms.id)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("pruneOldestDatapoints: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}
