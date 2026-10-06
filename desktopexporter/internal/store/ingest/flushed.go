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
// SweepOrphans is the only dictionary-row deleter and invalidates this cache.
// If precise invalidation fails, forgetting extra IDs is safe because their
// inserts conflict; retaining a deleted ID would create a dangling reference.
// A nil *FlushedIDs disables the optimization.
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

// LoadFlushedIDs builds a cache from every retained dictionary ID.
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

// Forget clears the cache when SweepOrphans cannot identify every deleted ID.
func (f *FlushedIDs) Forget() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.ids)
}

// forgetIDs removes IDs confirmed deleted by every completed sweep statement.
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
