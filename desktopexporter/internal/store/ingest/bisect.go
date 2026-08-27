package ingest

import (
	"context"
	"errors"
)

// ErrNotRowFault marks a failure that no individual row caused, so bisection
// must surface it rather than search for a culprit.
//
// # Why this has to be said explicitly
//
// BisectingWrite is a search, and it rests on one assumption: remove the guilty
// item and the rest succeeds. That is true of a duplicate key and false of a
// fault belonging to the operation itself, which fails identically for every
// subset. Given one of those, the search does not report it -- it halves all
// the way down, finds every single-row window failing, and returns "every row
// was rejected" with a nil error. The same shape as `git bisect` handed a test
// script that fails on every commit: it does not conclude the script is broken,
// it names an innocent commit with total confidence.
//
// The stakes are highest for the checks that exist to be loud. The pass
// mismatch guards in spans, logs and metrics fire when the two passes disagree
// about how many items exist -- a bug in our own code, whose whole purpose is
// to fail noisily instead of pairing every owner past that point with another
// owner's attributes. Run through an unguarded bisection, that alarm becomes a
// silent tally, and resilience machinery ends up eating the one error it was
// most important to hear.
//
// Wrap with %w. Anything not wrapped is still treated as row-specific and
// isolated, which is the right default for a fault we do not recognise.
var ErrNotRowFault = errors.New("not attributable to any single row")

// Rejected reports how many items an ingest could not write.
//
// A non-zero count is not an error. The batch succeeded; some items in it did
// not, and the caller is told how many so it can say so rather than leaving the
// sender to guess why its telemetry never appeared.
type Rejected struct {
	Count int
	// Reason is the store's message for the first item rejected. They are
	// usually all the same fault, and one concrete message beats a bare count:
	// "duplicate key" tells a sender that it is reusing ids.
	Reason error
}

// BisectingWrite writes as many of total items as it can, isolating the ones it
// cannot.
//
// # The problem it solves
//
// The appender path is all-or-nothing by construction. A constraint violation
// is only detected when the appender flushes, and a failed flush discards its
// entire buffer -- "appended and not yet flushed data has been invalidated".
// So a single unwritable row used to cost the whole batch: 599 good spans
// dropped because the 600th reused an id.
//
// # How
//
// Stop trusting a failure to be about the whole batch. Try all of it; if that
// fails, try each half; recurse. n items containing k bad ones costs O(k log n)
// attempts and lands the other n-k, while the common case -- k is zero -- is a
// single attempt with no overhead whatsoever. This is purely an error path.
//
// Bisection rather than a row-by-row INSERT fallback because it reuses the
// append path exactly as written, with no second copy of any column list to
// drift out of sync, and because it does not care *why* an item failed.
// Anything item-specific is isolated the same way. A fault that is not
// item-specific -- a full disk -- fails every half down to single items and
// surfaces as their rejection reason, which is the information the old code
// returned anyway, minus the loss of everything alongside it.
//
// # The contract attempt must honour
//
// attempt(lo, hi) writes items [lo, hi) and must be atomic: on failure it has
// to leave nothing behind, or the next attempt inherits rows it did not write.
// In practice that means running inside InTransaction. Each attempt is its own
// top-level transaction because DuckDB has neither nested transactions nor
// SAVEPOINT, so a failed half cannot be unwound inside an enclosing one.
//
// # Why an ordinal range and not a set of ids
//
// Ranges are what bisection halves, and a set of ids cannot express "the second
// half". Ordinals also keep two occurrences of a duplicated id separable, so
// the first can be written and the second rejected -- which an id-keyed skip
// set could not do. And an ordinal is an int, which is what lets this stay
// signal-agnostic: it knows nothing of spans, logs, metrics or columns.
func BisectingWrite(ctx context.Context, total int, attempt func(lo, hi int) error) (Rejected, error) {
	var rejected Rejected

	if total == 0 {
		// Still attempt once. An empty batch has to behave exactly as it did
		// before, including whatever the write path does with no rows.
		return rejected, attempt(0, 0)
	}

	// Iterative rather than recursive so a pathological batch cannot exhaust
	// the stack. Ranges still to try, taken newest-first.
	type window struct{ lo, hi int }
	todo := []window{{0, total}}

	for len(todo) > 0 {
		w := todo[len(todo)-1]
		todo = todo[:len(todo)-1]

		if err := ctx.Err(); err != nil {
			return rejected, err
		}

		err := attempt(w.lo, w.hi)
		if err == nil {
			continue
		}

		// A fault no row caused fails every half identically, so narrowing
		// would blame each row in turn and report none of it as an error.
		if errors.Is(err, ErrNotRowFault) {
			return rejected, err
		}

		if w.hi-w.lo == 1 {
			// Narrowed to one item and it still will not go in. This is the
			// row the batch was being thrown away for.
			//
			// Unless the context just went away underneath it: a cancelled
			// attempt says nothing about the row it happened to be carrying,
			// and recording it would report a rejection for work that was
			// never really tried. The loop head catches this too, but only
			// after the count has already been taken.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return rejected, ctxErr
			}
			rejected.Count++
			if rejected.Reason == nil {
				rejected.Reason = err
			}
			continue
		}

		mid := w.lo + (w.hi-w.lo)/2
		// High half pushed first so the low half is popped first, keeping
		// items in their original order. That makes "the first occurrence of a
		// duplicated id is the one that survives" true rather than incidental.
		todo = append(todo, window{mid, w.hi}, window{w.lo, mid})
	}

	return rejected, nil
}
