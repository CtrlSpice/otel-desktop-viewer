package ingest

import (
	"context"
	"errors"
	"maps"
	"slices"
)

// ErrNotRowFault marks a failure no single row caused. BisectingWrite surfaces
// these instead of searching: they fail identically for every subset, so
// narrowing would blame each row in turn and return no error at all. Wrap with %w.
var ErrNotRowFault = errors.New("not attributable to any single row")

// Rejection is one item an ingest would not write, identified by its ordinal in
// the batch walk so the caller can find the payload again.
type Rejection struct {
	Ordinal int
	Reason  error
}

// Rejected reports the items an ingest could not write, in walk order. A
// non-empty Rejected is not an error: the batch landed without them.
//
// Both ways an item can be refused end up here, and only here. Rows skipped
// before the write because the store already holds them, and rows bisection
// discovered by failing, are the same kind of fact about the same batch, so a
// caller counting or reporting them should not have to know which path found
// it. Keeping two tallies meant the count came from one place and the reason
// from another, and a batch refused for two different reasons described itself
// with whichever one happened to be asked.
type Rejected struct {
	Items []Rejection
}

// Count is how many items were refused.
func (r Rejected) Count() int { return len(r.Items) }

// Reason is why the first refused item was refused, for a one-line summary.
// Nil when nothing was refused.
func (r Rejected) Reason() error {
	if len(r.Items) == 0 {
		return nil
	}
	return r.Items[0].Reason
}

// Reasons counts the distinct reasons, so a caller can say "412 already stored,
// 1 constraint violation" rather than picking one and dropping the rest.
func (r Rejected) Reasons() map[string]int {
	out := map[string]int{}
	for _, item := range r.Items {
		out[item.Reason.Error()]++
	}
	return out
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
// preRejected names items the caller already knows it will not write, keyed by
// ordinal. They are recorded here rather than added by the caller afterwards so
// that every rejection is built in one place and lands in walk order. attempt
// is expected to skip them, which makes a range of nothing but pre-rejected
// items succeed trivially -- so bisection never blames them either.
func BisectingWrite(ctx context.Context, total int, preRejected map[int]error, attempt func(lo, hi int) error) (Rejected, error) {
	discovered := make(map[int]error, len(preRejected))
	maps.Copy(discovered, preRejected)

	collect := func() Rejected {
		ordinals := slices.Sorted(maps.Keys(discovered))
		out := Rejected{Items: make([]Rejection, 0, len(ordinals))}
		for _, o := range ordinals {
			out.Items = append(out.Items, Rejection{Ordinal: o, Reason: discovered[o]})
		}
		return out
	}

	if total == 0 {
		return collect(), attempt(0, 0)
	}

	// Iterative so a huge batch cannot exhaust the stack.
	type window struct{ lo, hi int }
	todo := []window{{0, total}}

	for len(todo) > 0 {
		w := todo[len(todo)-1]
		todo = todo[:len(todo)-1]

		if err := ctx.Err(); err != nil {
			return collect(), err
		}

		err := attempt(w.lo, w.hi)
		if err == nil {
			continue
		}

		// Fails the same for every subset, so searching would blame innocents.
		if errors.Is(err, ErrNotRowFault) {
			return collect(), err
		}

		if w.hi-w.lo == 1 {
			// Narrowed to one item that still will not go in -- unless we were
			// cancelled, which says nothing about the row it was carrying.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return collect(), ctxErr
			}
			discovered[w.lo] = err
			continue
		}

		// High half pushed first so the low half pops first, keeping items in
		// order: the first occurrence of a duplicated id is the one that lands.
		mid := w.lo + (w.hi-w.lo)/2
		todo = append(todo, window{mid, w.hi}, window{w.lo, mid})
	}

	return collect(), nil
}
