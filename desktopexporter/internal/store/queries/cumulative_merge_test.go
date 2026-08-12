package queries_test

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Differential harness for the cumulative histogram merge.
//
// A cumulative datapoint is a running total, so the activity within a time
// bucket is last-minus-first, not a sum. Get that subtly wrong and the result
// is not an error -- it is a plausible-looking chart with the wrong quantiles.
// Delta could be checked against real data; there is no cumulative histogram in
// any capture we have, so it is checked against an independent implementation
// of the same rules instead.
//
// The Go reference below is deliberately written from the specification rather
// than transcribed from the SQL, so that a shared misreading is the only way
// both can be wrong together.

type expBuckets struct {
	scale  int
	offset int
	counts []int64
}

// reference computes last-minus-first the way the spec requires: align both to
// a common scale and origin, subtract element-wise, and treat any negative
// result as a counter reset, in which case the activity is the later slice
// itself.
func reference(first, last expBuckets) (expBuckets, bool) {
	target := first.scale
	if last.scale < target {
		target = last.scale
	}
	f := downscale(first, first.scale-target)
	l := downscale(last, last.scale-target)

	lo, hi := span(f, l)
	if lo > hi {
		return expBuckets{scale: target, offset: 0}, false
	}
	out := make([]int64, 0, hi-lo)
	for i := lo; i < hi; i++ {
		d := at(l, i) - at(f, i)
		if d < 0 {
			return l, true // reset: the later slice is the activity
		}
		out = append(out, d)
	}
	return expBuckets{scale: target, offset: lo, counts: out}, false
}

func downscale(b expBuckets, levels int) expBuckets {
	if levels <= 0 || len(b.counts) == 0 {
		if levels > 0 {
			return expBuckets{scale: b.scale - levels, offset: floorDiv(b.offset, 1<<levels)}
		}
		return b
	}
	factor := 1 << levels
	newOffset := floorDiv(b.offset, factor)
	last := floorDiv(b.offset+len(b.counts)-1, factor)
	out := make([]int64, last-newOffset+1)
	for i, c := range b.counts {
		out[floorDiv(b.offset+i, factor)-newOffset] += c
	}
	return expBuckets{scale: b.scale - levels, offset: newOffset, counts: out}
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func span(a, b expBuckets) (int, int) {
	lo, hi := 1<<30, -(1 << 30)
	for _, x := range []expBuckets{a, b} {
		if len(x.counts) == 0 {
			continue
		}
		if x.offset < lo {
			lo = x.offset
		}
		if x.offset+len(x.counts) > hi {
			hi = x.offset + len(x.counts)
		}
	}
	return lo, hi
}

func at(b expBuckets, i int) int64 {
	j := i - b.offset
	if j < 0 || j >= len(b.counts) {
		return 0
	}
	return b.counts[j]
}

func literal(c []int64) string {
	if len(c) == 0 {
		return "[]::bigint[]"
	}
	parts := make([]string, len(c))
	for i, v := range c {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]::bigint[]"
}

// TestCumulativeMergeMatchesReference drives random cumulative pairs through
// the SQL macros and the Go reference and requires them to agree.
func TestCumulativeMergeMatchesReference(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range queries.Macros() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}

	rng := rand.New(rand.NewSource(20260811))
	cases, resets := 0, 0

	for i := 0; i < 300; i++ {
		scaleF := rng.Intn(6)
		scaleL := scaleF
		if rng.Intn(3) == 0 { // sometimes the SDK downscales mid-window
			scaleL = scaleF - 1 - rng.Intn(2)
		}
		offF := rng.Intn(13) - 6
		nF := rng.Intn(5) + 1

		f := expBuckets{scale: scaleF, offset: offF, counts: make([]int64, nF)}
		for j := range f.counts {
			f.counts[j] = int64(rng.Intn(50))
		}

		// `last` is built *from* `first` by adding non-negative increments,
		// because that is what a cumulative counter does. Generating the two
		// independently made almost every case a reset -- 238 of 300 -- which
		// exercised the clamp thoroughly and the subtraction hardly at all.
		//
		// The range may widen, since new observations can fall outside the
		// buckets seen so far.
		widenLeft, widenRight := rng.Intn(3), rng.Intn(3)
		offL := offF - widenLeft
		nL := nF + widenLeft + widenRight
		l := expBuckets{scale: scaleF, offset: offL, counts: make([]int64, nL)}
		for j := range l.counts {
			idx := offL + j
			l.counts[j] = at(f, idx) + int64(rng.Intn(40))
		}

		// A deliberate minority are genuine restarts, where every bucket drops.
		if rng.Intn(5) == 0 {
			for j := range l.counts {
				l.counts[j] = int64(rng.Intn(3))
			}
		}

		// Downscaling `last` after the fact keeps the scale-drift coverage
		// without breaking the growth relationship.
		if scaleL != scaleF {
			l = downscale(l, scaleF-scaleL)
		}

		want, wasReset := reference(f, l)
		if wasReset {
			resets++
		}
		cases++

		// The SQL composition under test: downscale both, align, subtract.
		var gotOffset sql.NullInt64
		var gotCounts sql.NullString
		query := fmt.Sprintf(`
			with t as (
				select
					downscale_exp_buckets(%s, %d, %d) as f,
					downscale_exp_buckets(%s, %d, %d) as l
			),
			a as (
				select
					case when len(f.counts) > 0 and len(l.counts) > 0 then least(f.offset, l.offset)
					     when len(f.counts) > 0 then f.offset
					     else l.offset end as target,
					f, l
				from t
			)
			select target,
			       diff_bucket_vectors(
			           pad_left_to_offset(l.counts, l.offset, target),
			           pad_left_to_offset(f.counts, f.offset, target)
			       )::varchar
			from a`,
			literal(f.counts), f.offset, f.scale-min(f.scale, l.scale),
			literal(l.counts), l.offset, l.scale-min(f.scale, l.scale))
		require.NoError(t, db.QueryRow(query).Scan(&gotOffset, &gotCounts), "case %d", i)

		if wasReset {
			// The reference clamps; SQL signals it by returning NULL.
			assert.Falsef(t, gotCounts.Valid,
				"case %d: expected a reset signal, got %s", i, gotCounts.String)
			continue
		}
		require.Truef(t, gotCounts.Valid, "case %d: unexpected reset signal", i)
		assert.Equalf(t, int64(want.offset), gotOffset.Int64, "case %d: offset", i)
		assert.Equalf(t, literalOf(want.counts), gotCounts.String, "case %d: counts", i)
	}
	t.Logf("compared %d cumulative pairs, %d of them counter resets", cases, resets)
	assert.Positive(t, resets, "the generator must produce resets or the clamp is untested")
}

func literalOf(c []int64) string {
	if len(c) == 0 {
		return "[]"
	}
	parts := make([]string, len(c))
	for i, v := range c {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
