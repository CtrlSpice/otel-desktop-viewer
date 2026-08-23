// Package storetest builds stores for tests.
//
// Every test package under store/ needs the same thing to start: an in-memory
// store, a context, and a guarantee it gets closed. That was six identical
// copies of the same six lines, one per package, which is the kind of
// duplication that stays invisible until one copy needs to change.
//
// The store package's own tests are not among the callers and cannot be. Its
// tests are an internal test package (`package store`), so importing a helper
// that imports store would be an import cycle. Its copy stays where it is on
// purpose -- it is not an oversight to tidy away.
package storetest

import (
	"context"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// New returns an in-memory store and a context for it. The store is closed
// when the test and its subtests finish.
//
// Cleanup is registered rather than returned. The teardown func it replaces
// was correct but had to be deferred by hand at all 99 call sites, and a
// forgotten defer leaks a DuckDB instance for the rest of the run without
// failing anything.
//
// Each call builds its own database. That is what lets these tests run in
// parallel, and it is not merely an optimisation they tolerate: many of them
// assert over whole-table state -- "there are exactly five metrics", "the
// stream table is empty after delete" -- which is only meaningful when nothing
// else is writing to the same database.
func New(t *testing.T) (*store.Store, context.Context) {
	t.Helper()

	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	return s, ctx
}
