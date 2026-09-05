package ingest

import (
	"context"
	"database/sql"
	"fmt"

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

// liveAttributeIDs is every attribute id currently referenced by anything.
//
// This is the price of the dictionary: DuckDB cannot put a foreign key into a
// LIST, so there is no anti-join to run and no refcount to consult. The live set
// has to be rebuilt by unnesting all ten owner arrays.
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
	union select unnest(attribute_ids) from metric_series
	union select unnest(metadata_ids) from metric_ingests
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
//
// Each carries `returning id::varchar` so SweepOrphans can tell FlushedIDs
// exactly which ids it deleted. The cast matters: scanning a uuid column
// straight into a Go string via database/sql yields the raw 16 bytes, not hex
// text -- DuckDB only formats it as text when asked to, which the cast does.
var sweepQueries = []string{
	// A resource or scope is live if any signal still points at it. All three
	// references are NOT NULL, so no null-safety dance is needed.
	`delete from resources where id not in (
		select resource_id from spans
		union select resource_id from logs
		union select resource_id from metric_ingests
		union select resource_id from metric_series
	) returning id::varchar`,
	`delete from scopes where id not in (
		select scope_id from spans
		union select scope_id from logs
		union select scope_id from metric_ingests
	) returning id::varchar`,

	`delete from attributes where id not in (` + liveAttributeIDs + `) returning id::varchar`,

	// Bounds are referenced only by datapoints, and through a real foreign
	// key rather than a LIST -- but the sweep still has to run, because a
	// foreign key stops a bad delete, it does not collect a row nothing
	// points at. bounds_id is nullable (only explicit-bucket histograms carry
	// one), hence the is-not-null guard on the live set.
	`delete from histogram_bounds where id not in (
		select bounds_id from datapoints where bounds_id is not null
	) returning id::varchar`,
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
//
// flushed may be nil when no store-level cache is in play.
//
// This is the only function that deletes dictionary rows, which is what makes
// a "these ids exist" cache safe at all -- so invalidation belongs here rather
// than at the call sites, where it would be four places to get it right
// instead of one. Each statement's `returning id` means this can invalidate
// precisely: only the ids a delete actually removed come out of FlushedIDs,
// so a resource still referenced by logs after traces are cleared stays
// marked, and the next batch that repeats it skips the insert exactly as
// before the sweep ran.
//
// That precision is deliberately not trusted blindly. If collecting the
// returned ids fails partway -- a query error, a scan error -- this falls
// back to flushed.Forget(), which drops the whole cache rather than a partial
// one. The asymmetry is intentional: being coarse is always safe, since a
// forgotten-but-still-live row just gets re-inserted and conflicts for free.
// Being precise but incomplete is not safe -- it leaves a stale "this row
// exists" entry for an id whose row is actually gone, an owner ends up
// referencing a deleted row, no FK can catch it, attrs_json silently joins
// nothing, and the attribute vanishes from the UI with no error anywhere.
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
