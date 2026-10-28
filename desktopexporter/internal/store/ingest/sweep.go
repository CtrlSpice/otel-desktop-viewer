package ingest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"

	"github.com/duckdb/duckdb-go/v2"
)

// ExecContext is the slice of *sql.DB and *sql.Tx that SweepOrphans needs.
//
// Taken as an interface so both the per-signal Clear paths (which hold a
// *sql.DB) and retention (package store, same) can call it without either
// importing the other.
//
// QueryContext rather than plain Exec: every sweep statement carries
// `returning id`, which SweepOrphans reads back so it can tell FlushedIDs
// exactly which ids it removed instead of forgetting the whole cache on every
// call. *sql.DB and *sql.Tx both already satisfy this.
type ExecContext interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Only shared histogram bounds outlive their owning datapoints. Signal
// attributes, resources and scopes are deleted with their rows.
var sweepQueries = []string{
	// Bounds are referenced only by datapoints, and through a real foreign
	// key rather than a LIST -- but the sweep still has to run, because a
	// foreign key stops a bad delete, it does not collect a row nothing
	// points at. bounds_id is nullable (only explicit-bucket histograms carry
	// one), hence the is-not-null guard on the live set.
	`delete from histogram_bounds where id not in (
		select bounds_id from metric_datapoints where bounds_id is not null
	) returning id::varchar`,
}

// SweepOrphans deletes histogram bounds no retained datapoint references.
//
// Retention sweeps before measuring so orphaned rows cannot trigger deletion of
// live telemetry, and after pruning to collect newly orphaned rows.
//
// flushed may be nil when no store-level cache is in play.
//
// Returned IDs invalidate FlushedIDs precisely. Any query or scan failure
// clears the whole cache because retaining a deleted ID is unsafe.
func SweepOrphans(ctx context.Context, exec ExecContext, flushed *FlushedIDs) error {
	var removed []duckdb.UUID
	for _, q := range sweepQueries {
		ids, err := collectDeletedIDs(ctx, exec, q)
		if err != nil {
			flushed.Forget()
			return fmt.Errorf("SweepOrphans: %w: %w", ErrIngestInternal, err)
		}
		removed = append(removed, ids...)
	}
	flushed.forgetIDs(removed)
	return nil
}

// SweepOrphansConn is the driver.Conn form used by ingestion cleanup before the
// store releases its write connection. It applies the same sweep and cache
// invalidation as SweepOrphans.
func SweepOrphansConn(ctx context.Context, conn driver.Conn, flushed *FlushedIDs) error {
	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		flushed.Forget()
		return fmt.Errorf("SweepOrphansConn: %w: connection cannot query", ErrIngestInternal)
	}
	var removed []duckdb.UUID
	for _, q := range sweepQueries {
		rows, err := queryer.QueryContext(ctx, q, nil)
		if err != nil {
			flushed.Forget()
			return fmt.Errorf("SweepOrphansConn: %w: %w", ErrIngestInternal, err)
		}
		for {
			dest := []driver.Value{nil}
			err := rows.Next(dest)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				rows.Close()
				flushed.Forget()
				return fmt.Errorf("SweepOrphansConn: %w: %w", ErrIngestInternal, err)
			}
			id, err := parseUUID(dest[0].(string))
			if err != nil {
				rows.Close()
				flushed.Forget()
				return fmt.Errorf("SweepOrphansConn: %w: %w", ErrIngestInternal, err)
			}
			removed = append(removed, id)
		}
		rows.Close()
	}
	flushed.forgetIDs(removed)
	return nil
}

// collectDeletedIDs runs one `delete ... returning id::varchar` statement and
// parses back exactly which ids it removed.
func collectDeletedIDs(ctx context.Context, exec ExecContext, query string) ([]duckdb.UUID, error) {
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []duckdb.UUID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		id, err := parseUUID(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
