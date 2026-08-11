package ingest

import (
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
// # Why a cache is safe here specifically
//
// A cache of "rows that exist" is only wrong when something deletes one. Every
// other table in this schema is deleted from several places -- spans alone go
// through retention, Clear, DeleteSpansByIDs and DeleteSpansByTraceIDs -- and a
// cache over those would have four chances to drift.
//
// Dictionary rows have exactly one deleter: SweepOrphans. So invalidation lives
// inside that function rather than at its call sites, and there is no way to
// remove a row without clearing the set in the same breath.
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
// Called by SweepOrphans, which is the only thing that deletes dictionary rows.
// Deliberately coarse: the sweep does not report which ids it removed, and
// re-establishing the whole set costs one flush per sender rather than risking a
// partial invalidation that misses one.
func (f *FlushedIDs) Forget() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.ids)
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
