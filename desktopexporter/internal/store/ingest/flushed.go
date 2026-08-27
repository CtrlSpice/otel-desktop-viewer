package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/duckdb/duckdb-go/v2"
)

// FlushedIDs remembers which dictionary rows are already in the database, so a
// batch whose attributes, resource and scope have all been seen before can skip
// its insert entirely.
//
// # Why this exists
//
// Flush issues three `insert ... on conflict do nothing` statements per OTLP
// batch. After the first few batches from a sender those insert nothing at all:
// the resource is identical, the scope is identical, and the attributes come
// from a small recurring set (the full race capture holds 488 distinct
// attributes, 24 resources and 1 scope). DuckDB still parses, plans,
// materialises the tuples, probes the index per row and commits -- about 2.3ms,
// and crucially that cost is per *batch*, not per span.
//
// That fixed cost is what made small batches pathological. Measured, spans/sec
// against the pre-dictionary schema:
//
//	batch size   old schema   dictionary   dictionary + this cache
//	        10        7,534        2,826                    16,012
//	        25       12,860        6,254                    33,550
//	        50       17,110       10,532                    54,516
//	       250       17,531       40,652                   103,472
//
// Small batches are the normal case for a desktop viewer watching one local
// service, where the batch processor flushes on its timeout rather than on size.
//
// That table was measured replaying a fixed capture, which is this cache's
// best case: a finite capture has a finite attribute set, so the hit rate
// climbs toward 100% and stays there. Live traffic keeps minting new triples
// -- a fresh url.path, a new db.statement, a stack trace that has never
// appeared before -- so a running collector's hit rate, and the speedup that
// comes with it, is lower than the table above and does not generalise from
// it.
//
// # Why a cache is safe here specifically
//
// A cache of "rows that exist" is only wrong when something deletes one. Every
// other table in this schema is deleted from several places -- spans alone go
// through retention, Clear and DeleteSpansByTraceIDs -- and a
// cache over those would have four chances to drift.
//
// Dictionary rows have exactly one deleter: SweepOrphans. So invalidation lives
// inside that function rather than at its call sites. It can afford to be
// precise -- forgetting exactly the ids a sweep actually removed, via
// forgetIDs -- because there is still only one place that ever has to get it
// right; see SweepOrphans and Forget for the coarse fallback it keeps for when
// precision cannot be trusted.
//
// # Failure mode, and why it is contained
//
// Stale-in the dangerous direction means an owner's uuid[] references a row that
// is gone. Nothing catches that: DuckDB cannot foreign-key into a LIST, attrs_json
// simply joins nothing, and the attribute disappears from the UI with no error.
// TestDictionaryIntegrityAcrossClearAndReingest exists for exactly this and is
// mutation-checked against an uninvalidated cache.
//
// Stale in the other direction -- forgetting a row that does exist -- is free:
// the insert runs and conflicts, which is today's behaviour.
//
// A nil *FlushedIDs disables caching entirely, so any caller that has no store
// to hand (tests, one-off tooling) gets correct-but-slower rather than wrong.
//
// # Cost
//
// 16 bytes per distinct id. The full race capture is 513 ids, about 8KB; a
// pathological million-attribute store is 16MB, bounded by the same retention
// that bounds the database copy.
type FlushedIDs struct {
	mu  sync.Mutex
	ids map[duckdb.UUID]struct{}
}

func NewFlushedIDs() *FlushedIDs {
	return &FlushedIDs{ids: make(map[duckdb.UUID]struct{})}
}

// idQueryer is the read access LoadFlushedIDs needs. *sql.DB satisfies it, so
// callers pass the store's pool directly.
type idQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// LoadFlushedIDs builds a FlushedIDs warmed from every id already on disk.
//
// Without this, opening a persistent --db that already holds the whole
// dictionary would still re-insert everything until repeated flushes refilled
// an empty in-process cache -- the fixed per-batch cost FlushedIDs exists to
// remove, paid all over again on every restart. This loads every id ever
// stored across the three dictionary tables, which sounds unbounded but is
// not: it is bounded the same way the on-disk dictionary itself is, by
// retention.
//
// A fresh or in-memory store has nothing to load, so this just runs three
// queries against empty tables and returns an empty cache -- equivalent to
// NewFlushedIDs.
func LoadFlushedIDs(ctx context.Context, db idQueryer) (*FlushedIDs, error) {
	f := NewFlushedIDs()
	for _, table := range [...]string{"attributes", "resources", "scopes", "histogram_bounds"} {
		if err := warmFlushedFrom(ctx, db, table, f); err != nil {
			return nil, fmt.Errorf("LoadFlushedIDs: %w: %w", ErrIngestInternal, err)
		}
	}
	return f, nil
}

// warmFlushedFrom reads every id out of one dictionary table and records it.
//
// Cast to varchar in the query rather than scanning the uuid column directly:
// scanning a uuid straight into a Go string via database/sql yields the raw
// 16 bytes, not hex text, because the driver's text formatting only happens
// when DuckDB is asked to produce text. The cast asks for exactly that.
func warmFlushedFrom(ctx context.Context, db idQueryer, table string, f *FlushedIDs) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select id::varchar from %s`, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []duckdb.UUID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		id, err := parseUUID(s)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.ids[id] = struct{}{}
	}
	return nil
}

// unseen returns the subset of m whose keys are not already recorded.
//
// It does not mark them: marking happens only after the insert actually
// succeeds, so a failed flush leaves the set untouched and the next batch
// retries the write.
func unseen[T any](f *FlushedIDs, m map[duckdb.UUID]T) map[duckdb.UUID]T {
	if f == nil || len(m) == 0 {
		return m
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[duckdb.UUID]T, len(m))
	for k, v := range m {
		if _, ok := f.ids[k]; !ok {
			out[k] = v
		}
	}
	return out
}

// mark records ids as present in the database. Called only on a successful
// insert.
func mark[T any](f *FlushedIDs, m map[duckdb.UUID]T) {
	if f == nil || len(m) == 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range m {
		f.ids[k] = struct{}{}
	}
}

// Forget drops everything, so the next flush rewrites whatever it needs.
//
// Called by SweepOrphans, which is the only thing that deletes dictionary
// rows -- but only as its fallback, when it could not safely determine
// exactly which ids a delete removed (a query or scan failure partway through
// the three sweep statements). Deliberately coarse for that case:
// re-establishing the whole set costs one flush per sender, which is cheap
// next to the alternative. A precise invalidation that misses even one id
// leaves a stale "this row exists" entry, and nothing catches that -- no FK,
// no error, just an attribute that silently stops appearing in the UI. Being
// coarse is always safe; being precise-but-incomplete is not. See forgetIDs
// for the precise path SweepOrphans uses when it can trust what it read back.
func (f *FlushedIDs) Forget() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.ids)
}

// forgetIDs drops exactly the given ids. Unexported: it is only safe when the
// caller actually knows what got deleted, which today only SweepOrphans can
// determine (via `delete ... returning id`), and only on the path where every
// sweep statement's returned rows were read successfully. Any doubt about
// that belongs on Forget's coarse path instead, never here.
func (f *FlushedIDs) forgetIDs(ids []duckdb.UUID) {
	if f == nil || len(ids) == 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		delete(f.ids, id)
	}
}

// Len reports how many ids are remembered. For tests and diagnostics.
func (f *FlushedIDs) Len() int {
	if f == nil {
		return 0
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ids)
}
