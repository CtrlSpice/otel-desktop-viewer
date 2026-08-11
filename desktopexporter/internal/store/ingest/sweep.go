package ingest

import (
	"context"
	"database/sql"
	"fmt"
)

// ExecContext is the slice of *sql.DB and *sql.Tx that SweepOrphans needs.
//
// Taken as an interface so both the per-signal Clear paths (which hold a
// *sql.DB) and retention (package store, same) can call it without either
// importing the other.
type ExecContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// liveAttributeIDs is every attribute id currently referenced by anything.
//
// This is the price of the dictionary: DuckDB cannot put a foreign key into a
// LIST, so there is no anti-join to run and no refcount to consult. The live set
// has to be rebuilt by unnesting all eight owner arrays.
//
// UNION rather than UNION ALL: the whole point is a distinct set, and letting
// DuckDB dedupe during the union is cheaper than materialising ~10^6 duplicate
// ids and deduping at the end.
const liveAttributeIDs = `
	select unnest(attribute_ids) as id from spans
	union select unnest(attribute_ids) from events
	union select unnest(attribute_ids) from links
	union select unnest(attribute_ids) from logs
	union select unnest(attribute_ids) from datapoints
	union select unnest(attribute_ids) from exemplars
	union select unnest(attribute_ids) from resources
	union select unnest(attribute_ids) from scopes`

// sweepQueries run in this order for a reason: resources and scopes go first,
// attributes last.
//
// Resources and scopes are themselves owners of attribute ids, so a resource
// deleted *after* the attributes sweep keeps its attributes alive through that
// sweep and leaves them behind for the next one -- correct, but a pass behind.
// Deleting the owners first means their attributes are already unreferenced when
// the dictionary sweep runs, so one pass collects everything.
//
// The failure mode of stopping halfway is orphan dictionary rows, never a
// dangling reference. That is the same direction ingest chose (attributes
// written before the owners that reference them), for the same reason: nothing
// enforces this, since DuckDB cannot put a foreign key into a LIST.
var sweepQueries = []string{
	// A resource or scope is live if any signal still points at it. All three
	// references are NOT NULL, so no null-safety dance is needed.
	`delete from resources where id not in (
		select resource_id from spans
		union select resource_id from logs
		union select resource_id from metric_ingests
	)`,
	`delete from scopes where id not in (
		select scope_id from spans
		union select scope_id from logs
		union select scope_id from metric_ingests
	)`,

	`delete from attributes where id not in (` + liveAttributeIDs + `)`,
}

// SweepOrphans deletes dictionary, resource and scope rows nothing references.
//
// It runs eagerly rather than lazily, and that is deliberate. Retention is
// size-driven: it measures the database and prunes the oldest telemetry until it
// fits. Orphaned rows count toward that measurement, so deferring the sweep
// means garbage inflates the number, retention deletes real spans to make room,
// and the garbage survives while the data does not. Worst in memory mode, where
// the measure is live heap.
//
// Callers: the three Clear paths, and retention -- once before its first size
// check (so pre-existing garbage can never be what pushes the store over the
// cap) and once at the end of every round (the prunes are what create the
// orphans).
func SweepOrphans(ctx context.Context, exec ExecContext) error {
	for _, q := range sweepQueries {
		if _, err := exec.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("SweepOrphans: %w: %w", ErrIngestInternal, err)
		}
	}
	return nil
}
