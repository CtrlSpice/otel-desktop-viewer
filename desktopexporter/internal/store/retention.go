package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	if s.dbPath == "" {
		var size int64
		err := s.db.QueryRowContext(ctx,
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
func (s *Store) EnforceRetention(ctx context.Context, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}

	for round := 0; round < maxPruneRounds; round++ {
		size, err := s.SizeBytes(ctx)
		if err != nil {
			return err
		}
		if size <= maxBytes {
			return nil
		}

		if err := s.pruneOldestSpans(ctx); err != nil {
			return err
		}
		if err := s.pruneOldestLogs(ctx); err != nil {
			return err
		}
		if err := s.pruneOldestDatapoints(ctx); err != nil {
			return err
		}

		// Checkpoint flushes the WAL and lets DuckDB reuse the freed blocks;
		// without it the file/memory measurement would not move.
		if _, err := s.db.ExecContext(ctx, `checkpoint`); err != nil {
			return fmt.Errorf("EnforceRetention: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}

// pruneCutoff returns the timestamp below which rows should be deleted,
// i.e. the pruneFraction percentile of the given time expression. Returns
// (0, false) when the table is empty.
func (s *Store) pruneCutoff(ctx context.Context, query string) (int64, bool, error) {
	var cutoff sql.NullInt64
	if err := s.db.QueryRowContext(ctx, query, pruneFraction).Scan(&cutoff); err != nil {
		return 0, false, fmt.Errorf("pruneCutoff: %w: %w", ErrRetentionInternal, err)
	}
	return cutoff.Int64, cutoff.Valid, nil
}

// pruneOldestSpans deletes the oldest fraction of spans along with their
// events, links, and attributes. Attribute rows for events and links carry
// the owning span_id (enforced by chk_attributes_one_owner), so a single
// span_id predicate covers all three owners. Leaves first, spans last.
func (s *Store) pruneOldestSpans(ctx context.Context) error {
	cutoff, ok, err := s.pruneCutoff(ctx,
		`select cast(quantile_cont(start_time, ?) as bigint) from spans`)
	if err != nil || !ok {
		return err
	}

	for _, q := range []string{
		`delete from attributes where span_id in (select span_id from spans where start_time < ?)`,
		`delete from links where span_id in (select span_id from spans where start_time < ?)`,
		`delete from events where span_id in (select span_id from spans where start_time < ?)`,
		`delete from spans where start_time < ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestSpans: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}

// pruneOldestLogs deletes the oldest fraction of logs and their attributes.
// Logs may arrive with timestamp = 0 (unset); observed_timestamp is the
// fallback, mirroring how GetStats computes lastReceived.
func (s *Store) pruneOldestLogs(ctx context.Context) error {
	const logTime = `coalesce(nullif(timestamp, 0), observed_timestamp)`

	cutoff, ok, err := s.pruneCutoff(ctx,
		`select cast(quantile_cont(`+logTime+`, ?) as bigint) from logs`)
	if err != nil || !ok {
		return err
	}

	for _, q := range []string{
		`delete from attributes where log_id in (select id from logs where ` + logTime + ` < ?)`,
		`delete from logs where ` + logTime + ` < ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestLogs: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}

// pruneOldestDatapoints deletes the oldest fraction of datapoints with their
// exemplars and attributes, then sweeps metric_ingests and metric_streams
// rows that no longer own any datapoints. The identity sweep matters:
// metric_ingests grows by one row per OTLP batch, so leaving orphans behind
// would let the store creep back over the cap with rows pruning can't touch.
// A swept stream that is still live gets recreated by ingest's find-or-insert.
func (s *Store) pruneOldestDatapoints(ctx context.Context) error {
	cutoff, ok, err := s.pruneCutoff(ctx,
		`select cast(quantile_cont(timestamp, ?) as bigint) from datapoints`)
	if err != nil || !ok {
		return err
	}

	doomed := `(select id from datapoints where timestamp < ?)`
	for _, q := range []string{
		`delete from attributes where exemplar_id in (select id from exemplars where datapoint_id in ` + doomed + `)`,
		`delete from attributes where datapoint_id in ` + doomed,
		`delete from exemplars where datapoint_id in ` + doomed,
		`delete from datapoints where timestamp < ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("pruneOldestDatapoints: %w: %w", ErrRetentionInternal, err)
		}
	}

	// Orphan sweep: ingest batches whose datapoints are all gone, then
	// streams whose ingest batches are all gone. Ordering matters for the
	// FK chain (attributes -> metric_ingests -> metric_streams).
	for _, q := range []string{
		`delete from attributes where metric_ingest_id in (
			select id from metric_ingests mi
			where not exists (select 1 from datapoints d where d.metric_ingest_id = mi.id)
		)`,
		`delete from metric_ingests mi
			where not exists (select 1 from datapoints d where d.metric_ingest_id = mi.id)`,
		`delete from metric_streams ms
			where not exists (select 1 from metric_ingests mi where mi.stream_id = ms.id)`,
	} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("pruneOldestDatapoints: %w: %w", ErrRetentionInternal, err)
		}
	}
	return nil
}
