package ingest

import (
	"context"
	"errors"
)

// ErrNotRowFault marks a failure no single row caused. BisectingWrite surfaces
// these instead of searching: they fail identically for every subset, so
// narrowing would blame each row in turn and return no error at all. Wrap with %w.
var ErrNotRowFault = errors.New("not attributable to any single row")

// Rejected reports items an ingest could not write. A non-zero Count is not an
// error: the batch landed without them.
type Rejected struct {
	Count int
	// Reason is why the first one was refused, so a sender can act on it.
	Reason error
}

// BisectingWrite writes as many of total items as it can, retrying a failed
// attempt in narrowing halves until each bad item is alone.
//
// A failed append discards its whole buffer, so without this one bad row costs
// the entire batch. k bad items in n costs O(k log n) attempts; k of zero is a
// single attempt, making this purely an error path.
//
// attempt(lo, hi) writes items [lo, hi) and must be atomic, or a failed try
// leaves rows the next one did not write. Ranges rather than ids because
// bisection halves ranges, and because two occurrences of one duplicated id
// stay separable.
func BisectingWrite(ctx context.Context, total int, attempt func(lo, hi int) error) (Rejected, error) {
	var rejected Rejected

	if total == 0 {
		return rejected, attempt(0, 0)
	}

	// Iterative so a huge batch cannot exhaust the stack.
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

		// Fails the same for every subset, so searching would blame innocents.
		if errors.Is(err, ErrNotRowFault) {
			return rejected, err
		}

		if w.hi-w.lo == 1 {
			// Narrowed to one item that still will not go in -- unless we were
			// cancelled, which says nothing about the row it was carrying.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return rejected, ctxErr
			}
			rejected.Count++
			if rejected.Reason == nil {
				rejected.Reason = err
			}
			continue
		}

		// High half pushed first so the low half pops first, keeping items in
		// order: the first occurrence of a duplicated id is the one that lands.
		mid := w.lo + (w.hi-w.lo)/2
		todo = append(todo, window{mid, w.hi}, window{w.lo, mid})
	}

	return rejected, nil
}
