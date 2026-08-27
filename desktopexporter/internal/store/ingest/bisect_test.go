package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attemptOver returns an attempt func that fails whenever its range contains
// any of bad, which is how a row-specific fault actually behaves.
func attemptOver(bad map[int]bool, attempts *int) func(lo, hi int) error {
	return func(lo, hi int) error {
		*attempts++
		for i := lo; i < hi; i++ {
			if bad[i] {
				return fmt.Errorf("row %d is bad", i)
			}
		}
		return nil
	}
}

func TestBisectingWrite_CleanBatchCostsOneAttempt(t *testing.T) {
	t.Parallel()
	var attempts int
	rep, err := ingest.BisectingWrite(t.Context(), 600, nil, attemptOver(map[int]bool{}, &attempts))
	require.NoError(t, err)
	assert.Zero(t, rep.Count())
	assert.Equal(t, 1, attempts, "the common case must not pay for the error path")
}

func TestBisectingWrite_IsolatesBadRows(t *testing.T) {
	t.Parallel()
	bad := map[int]bool{7: true, 293: true, 599: true}
	var attempts int
	rep, err := ingest.BisectingWrite(t.Context(), 600, nil, attemptOver(bad, &attempts))
	require.NoError(t, err, "row faults are reported, not returned")
	assert.Equal(t, 3, rep.Count())
	require.Error(t, rep.Reason())
	assert.Less(t, attempts, 100, "should be O(k log n), nowhere near one attempt per row")
}

// A fault no row caused must come back as an error rather than being reported
// as "every row was rejected".
//
// Without the sentinel this is the silent-corruption case: the guard that
// exists to fail loudly fires identically in every half, bisection blames each
// row in turn, and the call returns nil.
func TestBisectingWrite_SurfacesFaultsNoRowCaused(t *testing.T) {
	t.Parallel()
	structural := fmt.Errorf("%w: pass mismatch (spans 4/5)", ingest.ErrNotRowFault)

	var attempts int
	rep, err := ingest.BisectingWrite(t.Context(), 600, nil, func(lo, hi int) error {
		attempts++
		return structural
	})

	require.Error(t, err, "a structural fault must not be swallowed")
	assert.ErrorIs(t, err, ingest.ErrNotRowFault)
	assert.Contains(t, err.Error(), "pass mismatch", "the diagnosis must survive")
	assert.Zero(t, rep.Count(), "no row caused this, so no row should be blamed")
	assert.Equal(t, 1, attempts, "it fails the same for every subset; do not search")
}

// Cancellation is not a property of whichever row the search happened to be
// holding, so it must not be recorded as that row's rejection.
func TestBisectingWrite_CancellationIsNotARejection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	// The cancel has to land on a single-row window specifically. Anywhere
	// wider and the loop head catches it on the next pass; it is only at width
	// one that the code is about to write the failure down as a rejection.
	rep, err := ingest.BisectingWrite(ctx, 8, nil, func(lo, hi int) error {
		if hi-lo == 1 {
			cancel()
			return errors.New("interrupted")
		}
		return errors.New("force a split")
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, rep.Count(), "a cancelled attempt says nothing about its rows")
}

func TestBisectingWrite_EmptyBatchStillAttemptsOnce(t *testing.T) {
	t.Parallel()
	var attempts int
	rep, err := ingest.BisectingWrite(t.Context(), 0, nil, attemptOver(map[int]bool{}, &attempts))
	require.NoError(t, err)
	assert.Zero(t, rep.Count())
	assert.Equal(t, 1, attempts)
}

// The first occurrence of a repeated identity is the one that survives, which
// depends on ranges being attempted in order rather than incidentally.
func TestBisectingWrite_AttemptsRangesInOrder(t *testing.T) {
	t.Parallel()
	var order []int
	_, err := ingest.BisectingWrite(t.Context(), 4, nil, func(lo, hi int) error {
		if hi-lo == 1 {
			order = append(order, lo)
			return nil
		}
		return errors.New("force a split")
	})
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2, 3}, order)
}

// Both ways an item can be refused arrive through the same list, in walk order.
//
// Pre-rejected items used to be tallied on by the caller after the fact, so a
// batch refused for two different reasons reported the count from one place and
// the reason from the other -- and neither recorded which items they were.
func TestBisectingWrite_PreRejectedAndDiscoveredMergeInOrder(t *testing.T) {
	t.Parallel()
	preRejected := map[int]error{1: errAlready, 5: errAlready}

	rep, err := ingest.BisectingWrite(t.Context(), 8, preRejected, func(lo, hi int) error {
		// Pre-rejected items are skipped by the caller's own predicate, so a
		// range containing only those succeeds. Item 6 is genuinely bad.
		for i := lo; i < hi; i++ {
			if _, skipped := preRejected[i]; skipped {
				continue
			}
			if i == 6 {
				return errBad
			}
		}
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, 3, rep.Count())
	assert.Equal(t, []int{1, 5, 6}, ordinalsOf(rep), "walk order, regardless of which path found them")
	assert.ErrorIs(t, rep.Items[0].Reason, errAlready)
	assert.ErrorIs(t, rep.Items[2].Reason, errBad)

	// Every reason is kept, not just whichever was asked for first.
	assert.Equal(t, map[string]int{errAlready.Error(): 2, errBad.Error(): 1}, rep.Reasons())
}

var (
	errAlready = errors.New("already stored")
	errBad     = errors.New("constraint violation")
)

func ordinalsOf(r ingest.Rejected) []int {
	out := make([]int, 0, len(r.Items))
	for _, item := range r.Items {
		out = append(out, item.Ordinal)
	}
	return out
}
