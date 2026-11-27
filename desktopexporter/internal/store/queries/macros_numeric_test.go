package queries_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateBucketJSONUsesBoundsPresenceForRepresentation(t *testing.T) {
	db := macroDB(t)

	query := func(bounds, counts, scale, zeroCount, posCounts string) map[string]any {
		t.Helper()
		var raw string
		err := db.QueryRow(`select metric_aggregate_bucket_view_json(
			1::bigint, 0::bigint, 7::ubigint, 3.5::double,
			` + scale + `, 0.0::double, ` + zeroCount + `,
			{'offset': 0::integer, 'counts': ` + posCounts + `, 'folded': 0::bigint},
			{'offset': 0::integer, 'counts': []::ubigint[], 'folded': 0::bigint},
			` + bounds + `, ` + counts + `, null::json,
			bucket_extents(case when ` + bounds + ` is not null then hist_buckets(` + bounds + `, ` + counts + `)
				else exp_buckets(` + scale + `, 0, []::ubigint[], ` + zeroCount + `, 0, ` + posCounts + `) end)
		)::varchar`).Scan(&raw)
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &got))
		return got
	}

	t.Run("empty explicit bounds keep the catch-all bucket", func(t *testing.T) {
		got := query("[]::double[]", "[7]::ubigint[]", "null::integer",
			"null::ubigint", "[]::ubigint[]")

		require.IsType(t, []any{}, got["explicitBounds"])
		assert.Empty(t, got["explicitBounds"])
		assert.Equal(t, []any{float64(7)}, got["bucketCounts"])
		assert.NotContains(t, got, "min")
		assert.NotContains(t, got, "max")
		assert.NotContains(t, got, "scale")
		assert.NotContains(t, got, "positiveBucketCounts")
	})

	t.Run("nonempty explicit bounds stay explicit", func(t *testing.T) {
		got := query("[1.0]::double[]", "[3, 4]::ubigint[]", "null::integer",
			"null::ubigint", "[]::ubigint[]")

		assert.Equal(t, []any{float64(1)}, got["explicitBounds"])
		assert.Equal(t, []any{float64(3), float64(4)}, got["bucketCounts"])
		assert.Equal(t, float64(1), got["min"])
		assert.Equal(t, float64(1), got["max"])
		assert.NotContains(t, got, "scale")
	})

	t.Run("empty explicit observations omit unknowable extents", func(t *testing.T) {
		got := query("[1.0]::double[]", "[0, 0]::ubigint[]", "null::integer",
			"null::ubigint", "[]::ubigint[]")

		assert.NotContains(t, got, "min")
		assert.NotContains(t, got, "max")
	})

	t.Run("null bounds stay exponential", func(t *testing.T) {
		got := query("null::double[]", "null::ubigint[]", "0::integer",
			"2::ubigint", "[5]::ubigint[]")

		assert.NotContains(t, got, "explicitBounds")
		assert.NotContains(t, got, "bucketCounts")
		assert.Equal(t, float64(0), got["scale"])
		assert.Equal(t, float64(2), got["zeroCount"])
		assert.Equal(t, []any{float64(5)}, got["positiveBucketCounts"])
		assert.Equal(t, float64(0), got["min"])
		assert.Equal(t, float64(2), got["max"])
	})

	t.Run("empty exponential observations omit unknowable extents", func(t *testing.T) {
		got := query("null::double[]", "null::ubigint[]", "0::integer",
			"0::ubigint", "[0]::ubigint[]")

		assert.NotContains(t, got, "min")
		assert.NotContains(t, got, "max")
	})
}

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
	db := macroDB(t)

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

// TestDownscaleExpBuckets pins what downscaling an exponential histogram
// produces against hand-worked values.
//
// The clamped ends are the part worth testing. Only the first and last output
// buckets can be partial; every bucket between them is exactly 2^levels wide,
// so an off-by-one in the bounds shows up at the edges,
// which is why several cases below start at an offset that does not divide
// evenly.
func TestDownscaleExpBuckets(t *testing.T) {
	db := macroDB(t)

	cases := []struct {
		name   string
		counts string
		offset int
		levels int
		want   string // {'offset': N, 'counts': [...]}
	}{
		{
			// Aligned and even: adjacent pairs merge, nothing is clamped.
			name: "levels 1 merges pairs", counts: "[1,2,3,4]", offset: 0, levels: 1,
			want: "{'offset': 0, 'counts': [3, 7]}",
		},
		{
			// Four inputs per output at levels 2.
			name: "levels 2 merges fours", counts: "[1,1,1,1,1,1,1,1]", offset: 0, levels: 2,
			want: "{'offset': 0, 'counts': [4, 4]}",
		},
		{
			// Offset 3 at levels 1: index 3 lands in bucket 1 alone (its
			// partner, index 2, is not in the array), 4 and 5 fill bucket 2.
			// The first output bucket is clamped; the second is not.
			name: "odd offset clamps the first bucket", counts: "[1,1,1]", offset: 3, levels: 1,
			want: "{'offset': 1, 'counts': [1, 2]}",
		},
		{
			// Trailing partial bucket: 3 inputs at levels 1 leaves the last
			// output bucket holding one.
			name: "odd length clamps the last bucket", counts: "[5,6,7]", offset: 0, levels: 1,
			want: "{'offset': 0, 'counts': [11, 7]}",
		},
		{
			// Four output buckets from an offset that does not sit on a
			// 2^levels boundary: the interesting middle case, where the first
			// bucket is partial and the rest are full. Original indices 5..11
			// at levels 1 fall in buckets 2,3,3,4,4,5,5 -- so bucket 2 gets
			// only the count at index 5, and each later bucket gets a pair.
			name:   "several buckets from an unaligned offset",
			counts: "[1,2,3,4,5,6,7]", offset: 5, levels: 1,
			want: "{'offset': 2, 'counts': [1, 5, 9, 13]}",
		},
		{
			// Negative offsets are ordinary here -- exponential histogram
			// buckets below 1 have them -- and floor_div rounds toward
			// negative infinity, so -4/2 = -2 rather than truncating to -1.
			name: "negative offset", counts: "[1,2,3,4]", offset: -4, levels: 1,
			want: "{'offset': -2, 'counts': [3, 7]}",
		},
		{
			// Everything collapses into one bucket once levels exceeds the
			// span of the input.
			name: "levels beyond the span", counts: "[1,2,3]", offset: 0, levels: 10,
			want: "{'offset': 0, 'counts': [6]}",
		},
		{
			// No-op: levels <= 0 returns the input untouched, offset included.
			name: "levels zero is a no-op", counts: "[1,2,3]", offset: 7, levels: 0,
			want: "{'offset': 7, 'counts': [1, 2, 3]}",
		},
		{
			// An empty array still carries an offset, and that offset must be
			// rescaled: leaving it at the source scale drags the caller's
			// min() alignment point far below any real bucket, and the
			// zero-fill that follows is unbounded.
			name: "empty counts still rescales the offset", counts: "[]::bigint[]", offset: 8, levels: 2,
			want: "{'offset': 2, 'counts': []}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			q := fmt.Sprintf(
				"select downscale_exp_buckets(%s::bigint[], %d, %d)::varchar",
				tc.counts, tc.offset, tc.levels)
			require.NoError(t, db.QueryRow(q).Scan(&got))
			require.Equal(t, tc.want, got)
		})
	}
}

// Downscaling must conserve the total count across varied shapes.
func TestDownscaleConservesTotal(t *testing.T) {
	db := macroDB(t)

	var bad int
	require.NoError(t, db.QueryRow(`
		with shapes as (
			select
				(random() * 60)::int + 1         as n,
				((random() - 0.5) * 400)::bigint as off,
				(random() * 6)::int              as lev,
				(random() * 997)::bigint         as seed
			from range(3000)
		),
		built as (
			select list_transform(range(0, n), lambda i: ((seed + i * 11) % 17)::bigint) as counts,
			       off, lev
			from shapes
		)
		select count(*) from built
		where list_sum(counts) is distinct from
		      list_sum(downscale_exp_buckets(counts, off, lev).counts)
	`).Scan(&bad))
	require.Zero(t, bad, "downscaling must preserve the total count")
}

// TestBucketStartTypePrecision verifies that the bucket_start column produced
// by the adaptive-time-bucketing CTE stays as BIGINT and survives a
// ::varchar cast without scientific notation. DuckDB's `/` operator returns
// DOUBLE even for bigint operands; we use `//` (integer division) instead.
func TestBucketStartTypePrecision(t *testing.T) {
	db := macroDB(t)

	rows, err := db.Query(`
		with params as (
			select
				greatest(1000000::bigint,
					(4000000000000000000::bigint - 0::bigint) // 1000000000::bigint
				) as bucket_ns
		),
		bucketed as (
			select (d.ts // p.bucket_ns) * p.bucket_ns as bucket_start
			from params p,
				(values (1700000000000000000::bigint),
				        (1700000060000000000::bigint),
				        (1700000120000000000::bigint)) d(ts)
		)
		select typeof(bucket_start), bucket_start::varchar
		from bucketed order by bucket_start
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var typ, val string
		require.NoError(t, rows.Scan(&typ, &val))
		assert.Equal(t, "BIGINT", typ, "bucket_start must be BIGINT, not DOUBLE")
		assert.NotContains(t, val, "e+", "varchar cast must not produce scientific notation")
	}
}

func TestJSONEachExtractsNestedValueKeys(t *testing.T) {
	db := macroDB(t)

	var keys string
	require.NoError(t, db.QueryRow(`
		select string_agg(json_extract_string(entry.value, '$.key'), ',' order by entry.key)
		from json_each(json('{"kind":"map","value":[{"key":"first","value":{"kind":"int64","value":"1"}},{"key":"second","value":{"kind":"array","value":[]}}]}'), '$.value') entry
	`).Scan(&keys))
	assert.Equal(t, "first,second", keys)
}

// TestListSliceBoundsAreAsymmetric pins DuckDB behaviour this schema depends on
// but does not control.
//
// list_slice clamps an out-of-range *end* and does not clamp an out-of-range
// *start*: a start below 1 yields an empty list. Two macros here compute slice
// bounds arithmetically from an offset that can be negative, so which of those
// two rules applies decides whether counts survive.
//
// This characterisation test records DuckDB behavior the macros depend on.
func TestListSliceBoundsAreAsymmetric(t *testing.T) {
	db := macroDB(t)

	q := func(sqlText string) string {
		var got string
		require.NoError(t, db.QueryRow(sqlText).Scan(&got))
		return got
	}

	// The end clamps. This is why `least(..., len(counts))` in
	// downscale_exp_buckets is defensive rather than load-bearing.
	require.Equal(t, "[1, 2, 3]", q("select list_slice([1,2,3], 1, 3)::varchar"))
	require.Equal(t, "[1, 2, 3]", q("select list_slice([1,2,3], 1, 4)::varchar"),
		"an end past the array should clamp to the last element")
	require.Equal(t, "[1, 2, 3]", q("select list_slice([1,2,3], 1, 99)::varchar"))

	// The start does not clamp below 1 -- it empties. This is why
	// downscale_exp_buckets wraps its start in greatest(..., 0), and why
	// fold_below_cutoff needs its cutoff >= offset_ guard.
	require.Equal(t, "[1, 2]", q("select list_slice([1,2,3], 0, 2)::varchar"),
		"a start of 0 is tolerated")
	require.Equal(t, "[]", q("select list_slice([1,2,3], -1, 2)::varchar"),
		"a start below 0 yields an empty list rather than clamping -- "+
			"this is the asymmetry the macros are written around")
}

// The guard, asserted rather than assumed: fold_below_cutoff never loses
// counts, whatever the cutoff. Its slice start is only >= 1 because the
// `cutoff < offset_` branch catches everything below, so this is the test that
// notices if that branch is ever relaxed.
func TestFoldBelowCutoffConservesCounts(t *testing.T) {
	db := macroDB(t)

	var bad int
	require.NoError(t, db.QueryRow(`
		with shapes as (
			select
				(random() * 30)::int + 1          as n,
				((random() - 0.5) * 200)::bigint  as off,
				((random() - 0.5) * 400)::bigint  as cut,
				(random() * 991)::bigint          as seed
			from range(3000)
		),
		built as (
			select list_transform(range(0, n), lambda i: ((seed + i * 5) % 9 + 1)::bigint) as counts,
			       off, cut
			from shapes
		),
		folded as (
			select counts, fold_below_cutoff(counts, off, cut) as r from built
		)
		select count(*) from folded
		where list_sum(counts)
		      is distinct from coalesce(list_sum(r.counts), 0) + r.folded
	`).Scan(&bad))
	require.Zero(t, bad,
		"folding must move counts into `folded`, never drop them")
}

func TestHugeintToDouble(t *testing.T) {
	// Independent rounding oracle: math/big, not the SQL's significand algorithm.
	limit := new(big.Int).Lsh(big.NewInt(1), 127)
	min := new(big.Int).Neg(limit)
	max := new(big.Int).Sub(limit, big.NewInt(1))
	var inputs []string
	seen := map[string]bool{}
	add := func(n *big.Int) {
		if n.Cmp(min) < 0 || n.Cmp(max) > 0 || seen[n.String()] {
			return
		}
		seen[n.String()] = true
		inputs = append(inputs, n.String())
	}
	for exponent := 0; exponent <= 127; exponent++ {
		power := new(big.Int).Lsh(big.NewInt(1), uint(exponent))
		for d := int64(-3); d <= 3; d++ {
			n := new(big.Int).Add(power, big.NewInt(d))
			add(n)
			add(new(big.Int).Neg(n))
		}
		if exponent < 53 || exponent == 127 {
			continue
		}
		// Halfway points after both even and odd retained significands, at
		// both ends of every binary64 binade, plus their immediate neighbours.
		step := new(big.Int).Lsh(big.NewInt(1), uint(exponent-52))
		for _, significand := range []int64{1 << 52, 1<<52 + 1, 1<<52 + 2, 1<<53 - 2, 1<<53 - 1} {
			mid := new(big.Int).Mul(big.NewInt(significand), step)
			mid.Add(mid, new(big.Int).Rsh(new(big.Int).Set(step), 1))
			for d := int64(-1); d <= 1; d++ {
				n := new(big.Int).Add(mid, big.NewInt(d))
				add(n)
				add(new(big.Int).Neg(n))
			}
		}
	}
	rng := rand.New(rand.NewSource(608))
	for range 512 {
		n := new(big.Int).Rand(rng, limit)
		add(n)
		add(new(big.Int).Neg(n))
	}
	want := make([]string, len(inputs))
	for i, input := range inputs {
		n, ok := new(big.Int).SetString(input, 10)
		require.True(t, ok)
		f, _ := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetInt(n).Float64()
		want[i] = fmt.Sprintf("%016x", math.Float64bits(f))
		parsed, err := strconv.ParseFloat(input, 64)
		require.NoError(t, err)
		require.Equal(t, math.Float64bits(f), math.Float64bits(parsed), input)
	}
	encoded, err := json.Marshal(inputs)
	require.NoError(t, err)
	rows, err := macroDB(t).Query(`select i, hugeint_to_double(v::hugeint)
		from unnest(from_json(?, '["VARCHAR"]')) with ordinality u(v, i)`, string(encoded))
	require.NoError(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		var i int
		var got float64
		require.NoError(t, rows.Scan(&i, &got))
		require.Equal(t, want[i-1], fmt.Sprintf("%016x", math.Float64bits(got)), inputs[i-1])
		count++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, len(inputs), count)
	var null sql.NullFloat64
	require.NoError(t, macroDB(t).QueryRow(`select hugeint_to_double(null::hugeint)`).Scan(&null))
	require.False(t, null.Valid)
	t.Logf("%d signed HUGEINT cases match math/big and strconv bitwise", count)

	t.Run("NumberBigInt", func(t *testing.T) {
		node, err := exec.LookPath("node")
		if err != nil {
			t.Skip("Node is unavailable; Go rounding oracles were checked")
		}
		cmd := exec.Command(node, "-e", `const fs = require('node:fs');
const inputs = JSON.parse(fs.readFileSync(0, 'utf8'));
console.log(JSON.stringify(inputs.map(s => {
  const b = Buffer.alloc(8); b.writeDoubleBE(Number(BigInt(s))); return b.toString('hex');
})));`)
		cmd.Stdin = bytes.NewReader(encoded)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		var got []string
		require.NoError(t, json.Unmarshal(out, &got))
		require.Equal(t, want, got)
		t.Logf("%d cases also match ECMAScript Number(BigInt) bitwise", count)
	})
}

func TestHistogramInterpolationIntegerBoundary(t *testing.T) {
	// These exact positive counts exposed native HUGEINT rounding errors.
	for _, count := range []string{
		"18446744073709553665", "36893488147419107329",
		"2417851639229258349412352", "2475880078570760549798248448",
		"81129638414606690702988259885057", "170141183460469231731687303715884105727",
	} {
		f, err := strconv.ParseFloat(count, 64)
		require.NoError(t, err)
		var linear, logarithmic float64
		require.NoError(t, macroDB(t).QueryRow(`select
			interp_linear(0.0, 1.0, 0::hugeint, ?::hugeint, 1.0::double),
			interp_loglin(1.0, 2.0, 0::hugeint, ?::hugeint, ?::double)`, count, count, f/2).
			Scan(&linear, &logarithmic))
		require.Equal(t, math.Float64bits(1/f), math.Float64bits(linear), count)
		require.Equal(t, math.Float64bits(math.Sqrt(2)), math.Float64bits(logarithmic), count)
	}
	var got float64
	require.NoError(t, macroDB(t).QueryRow(`select interp_linear(0.0, 1.0,
		'-9007199254740994'::hugeint, 1::hugeint, 0.0::double)`).Scan(&got))
	require.Equal(t, float64(9007199254740994), got)
}

func numericID(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func numericMetricDB(t *testing.T, kind string, seriesCount int) (*sql.DB, []string) {
	t.Helper()
	db := freshDB(t)
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`insert into metric_records(record_id,id,resource_id,resource_payload_id,scope_id,name,metric_type,aggregation_temporality,datapoints,resource,scope)
		 values ('` + numericID(4) + `','` + numericID(3) + `','` + numericID(1) + `','` + numericID(1) + `','` + numericID(2) + `','numerics','` + kind + `',1,[],
		 {'id': '` + numericID(1) + `'::uuid, 'payload_id': '` + numericID(1) + `'::uuid, 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0},
		 {'id': '` + numericID(2) + `'::uuid, 'name': '', 'version': '', 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0})`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}
	if kind == "Sum" {
		_, err := db.Exec("update metric_records set aggregation_temporality=2")
		require.NoError(t, err)
	}
	var ids []string
	// Preserve ascending series-ID order in the cancellation-sensitive fold
	// fixture while using the production deterministic identity expression.
	_, err := db.Exec(`create temp table numeric_series as select row_number() over(order by id)-1 as ordinal, *
		from (select range::varchar as key, md5(?::varchar||':'||range::varchar)::uuid as id from range(?))`, numericID(3), seriesCount)
	require.NoError(t, err)
	for i := range seriesCount {
		var id string
		err := db.QueryRow(`select id::varchar from numeric_series where ordinal=?`, i).Scan(&id)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return db, ids
}

func numericPoint(t *testing.T, db *sql.DB, id, series int, timestamp uint64, value float64, integer *int64) {
	t.Helper()
	var double any = value
	if integer != nil {
		double = nil
	}
	var key string
	require.NoError(t, db.QueryRow(`select key from numeric_series where ordinal=?`, series).Scan(&key))
	_, err := db.Exec(`update metric_records set datapoints=list_append(datapoints,
		struct_pack(id:=$1::uuid,timestamp:=$2::ubigint,attributes:={'series': $3::varchar}::variant,
		attribute_key:=$3::varchar,double_value:=$4::double,int_value:=$5::bigint,value_type:=$6::varchar,
		count:=1::ubigint,sum:=$7::double,explicit_bounds:=[0,1]::double[],bucket_counts:=[0,1,0]::ubigint[],
		scale:=0,zero_count:=0::ubigint,zero_threshold:=0::double,positive_bucket_offset:=0,
		positive_bucket_counts:=[1]::ubigint[],negative_bucket_offset:=0,negative_bucket_counts:=[]::ubigint[])),
		point_time_min=least(point_time_min,$2::ubigint),point_time_max=greatest(point_time_max,$2::ubigint)`,
		numericID(1000+id), timestamp, key,
		double, integer, map[bool]string{true: "Int", false: "Double"}[integer != nil], value)
	require.NoError(t, err)
}

func numericView(t *testing.T, db *sql.DB, selected []string, end uint64, views int64, aggregate bool, histogram bool) map[string]any {
	t.Helper()
	var raw json.RawMessage
	var err error
	var target int64
	var quantiles []float64
	if histogram {
		target, quantiles = 1, []float64{0.5}
	}
	if aggregate {
		raw, err = metrics.GetMetricAggregateView(context.Background(), db, numericID(3), store.BoundedTimeRange(0, end), target, nil, quantiles, 0, views, selected, "")
	} else {
		raw, err = metrics.GetMetricView(context.Background(), db, numericID(3), store.BoundedTimeRange(0, end), target, nil, quantiles, 0, views, 0, selected, "", nil, 0)
	}
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

// Reinsert the identical stored rows (including IDs) in a different physical
// order. Changing ingest UUIDs instead would change the timestamp-tie contract.
func reorderNumericPoints(t *testing.T, db *sql.DB, reverse bool) {
	t.Helper()
	order := "d.id"
	if reverse {
		order = "d.id desc"
	}
	_, err := db.Exec(`update metric_records set datapoints=(select list(d order by ` + order + `)
		from (select unnest(datapoints) as d))`)
	require.NoError(t, err)
}

func TestMetricNumericsOrderedScalarFolds(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(fmt.Sprintf("cross-series=%v", pool), func(t *testing.T) {
			n := 1
			if pool {
				n = 4
			}
			db, ids := numericMetricDB(t, "Sum", n)
			for i, value := range []float64{1e16, 1, -1e16, 1} {
				series, timestamp := 0, uint64(100)
				if pool {
					series = i
				}
				// Tied timestamps make the datapoint ID tie-break observable.
				numericPoint(t, db, i, series, timestamp, value, nil)
				if pool {
					numericPoint(t, db, 10+i, series, 200, 0, nil)
				}
			}
			var baseline map[string]any
			for _, reverse := range []bool{false, true} {
				reorderNumericPoints(t, db, reverse)
				for _, threads := range []int{1, 4} {
					_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
					require.NoError(t, err)
					full := numericView(t, db, ids, 999999, 1, false, false)
					only := numericView(t, db, ids, 999999, 1, true, false)
					require.Equal(t, full["scalarAggregate"], only["scalarAggregate"])
					if baseline != nil {
						require.Equal(t, baseline, full, "physical order=%v threads=%d", reverse, threads)
					}
					baseline = full
					pools := full["scalarAggregate"].(map[string]any)
					for _, key := range []string{"all", "selected"} {
						buckets := pools[key].([]any)
						require.Len(t, buckets, 1)
						bucket := buckets[0].(map[string]any)
						require.Equal(t, float64(1), bucket["sum"])
						count := float64(4)
						if pool {
							count = 8
							require.Equal(t, float64(-1000), bucket["rate"])
						}
						require.Equal(t, 1/count, bucket["avg"])
					}
					if !pool {
						series := full["timeseries"].([]any)[0].(map[string]any)
						require.Equal(t, float64(1), series["stats"].(map[string]any)["sum"])
						require.Equal(t, 0.25, series["stats"].(map[string]any)["avg"])
						bucket := series["views"].([]any)[0].(map[string]any)
						require.Equal(t, float64(1), bucket["sum"])
						require.Equal(t, 0.25, bucket["avg"])
					}
				}
			}
		})
	}
}

func TestMetricNumericsOrderedRates(t *testing.T) {
	for _, separateBuckets := range []bool{false, true} {
		db, ids := numericMetricDB(t, "Sum", 1)
		for i, value := range []int64{0, 10000000000000000, 10000000000000001, 1, 2} {
			timestamp := uint64(i * 100)
			if separateBuckets {
				timestamp = uint64(i) * 1e9
			}
			numericPoint(t, db, i, 0, timestamp, 0, &value)
		}
		for _, reverse := range []bool{false, true} {
			reorderNumericPoints(t, db, reverse)
			for _, threads := range []int{1, 4} {
				_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
				require.NoError(t, err)
				end, views := uint64(999999), int64(1)
				if separateBuckets {
					end, views = 4e9, 5
				}
				full := numericView(t, db, ids, end, views, false, false)
				series := full["timeseries"].([]any)[0].(map[string]any)
				want := 1000.0 // chronological deltas add to 1, divided by 1ms
				if separateBuckets {
					want = 0.25 // [1e16, 1, -1e16, 1] in bucket timestamp order
				} else {
					require.Equal(t, want, series["views"].([]any)[0].(map[string]any)["rate"])
				}
				require.Equal(t, want, series["rateStats"].(map[string]any)["avg"])
				only := numericView(t, db, ids, end, views, true, false)
				require.Equal(t, full["scalarAggregate"], only["scalarAggregate"])
			}
		}
	}
}

func TestMetricNumericsNegativeDeltaProjection(t *testing.T) {
	db, ids := numericMetricDB(t, "Sum", 1)
	for i, value := range []int64{0, -9007199254740994} {
		numericPoint(t, db, i, 0, uint64(i)*1e9, 0, &value)
	}
	full := numericView(t, db, ids, 1e9, 2, false, false)
	series := full["timeseries"].([]any)[0].(map[string]any)
	latest := series["datapoints"].([]any)[0].(map[string]any)
	require.Equal(t, "-9007199254740994", latest["delta"])
	require.Equal(t, "-9007199254740994", latest["intValue"])
	require.Equal(t, false, latest["isReset"])
	views := series["views"].([]any)
	got := views[len(views)-1].(map[string]any)["rate"].(float64)
	require.Equal(t, math.Float64bits(-9007199254740994), math.Float64bits(got))
}

func TestMetricNumericsOrderedHistogramSums(t *testing.T) {
	for _, kind := range []string{"Histogram", "ExponentialHistogram"} {
		t.Run(kind, func(t *testing.T) {
			db, ids := numericMetricDB(t, kind, 4)
			for i, value := range []float64{1e16, 1, -1e16, 1} {
				numericPoint(t, db, i, 0, 100, value, nil)
			}
			for i, value := range []float64{1e16, -1e16, 1} {
				numericPoint(t, db, 10+i, i+1, 100, value, nil)
			}
			// Histogram rows have no scalar arms.
			_, err := db.Exec("update metric_records set datapoints=list_transform(datapoints,lambda d:struct_update(d,double_value:=null,value_type:=null))")
			require.NoError(t, err)
			var baseline map[string]any
			for _, reverse := range []bool{false, true} {
				reorderNumericPoints(t, db, reverse)
				for _, threads := range []int{1, 4} {
					_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
					require.NoError(t, err)
					full := numericView(t, db, ids, 999999, 0, false, true)
					only := numericView(t, db, ids, 999999, 0, true, true)
					require.Equal(t, full["aggregate"], only["aggregate"])
					if baseline != nil {
						require.Equal(t, baseline, full)
					}
					baseline = full
					aggregate := full["aggregate"].([]any)
					require.Len(t, aggregate, 1)
					require.Equal(t, float64(1), aggregate[0].(map[string]any)["sum"])
					for _, raw := range full["timeseries"].([]any) {
						series := raw.(map[string]any)
						if series["seriesRef"] == ids[0] {
							points := series["datapoints"].([]any)
							require.Len(t, points, 1)
							require.Equal(t, float64(1), points[0].(map[string]any)["sum"])
						}
					}
				}
			}
		})
	}
}

func TestMetricNumericsHistogramVectorMerges(t *testing.T) {
	for _, kind := range []string{"Histogram", "ExponentialHistogram"} {
		for _, tc := range []struct {
			name               string
			vectors            [3]string
			first, second, all []string
		}{
			{"unequal lengths and NULL slots", [3]string{"[1,null,3]", "[4]", "[null,2]"}, []string{"5", "0", "3"}, []string{"", "2"}, []string{"5", "2", "3"}},
			{"empty vectors", [3]string{"[]", "[]", "[]"}, []string{}, []string{}, []string{}},
			{"absent vectors", [3]string{"null", "null", "null"}, []string{}, nil, []string{}},
			{"exact totals above uint64", [3]string{"[18446744073709551615,1]", "[1]", "[2,3]"}, []string{"18446744073709551616", "1"}, []string{"2", "3"}, []string{"18446744073709551618", "4"}},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				db, ids := numericMetricDB(t, kind, 2)
				for i, vector := range tc.vectors {
					series := 0
					if i == 2 {
						series = 1
					}
					numericPoint(t, db, i, series, 100, 0, nil)
					column := "bucket_counts"
					bounds := ""
					if kind == "ExponentialHistogram" {
						column, bounds = "positive_bucket_counts", ", explicit_bounds:=null"
					}
					_, err := db.Exec("update metric_records set datapoints=list_transform(datapoints,lambda d:case when d.id=?::uuid then struct_update(d,double_value:=null,value_type:=null,"+column+":="+vector+"::ubigint[]"+bounds+") else d end)", numericID(1000+i))
					require.NoError(t, err)
				}
				raw, err := metrics.GetMetricView(t.Context(), db, numericID(3), store.BoundedTimeRange(0, 1000), 1, nil, nil, 0, 0, 0, nil, "", nil, 0)
				require.NoError(t, err)
				var result struct {
					Timeseries []struct {
						SeriesRef  string `json:"seriesRef"`
						Datapoints []struct {
							Counts   []string `json:"bucketCounts"`
							Positive []string `json:"positiveBucketCounts"`
						} `json:"datapoints"`
					} `json:"timeseries"`
					Aggregate []struct {
						Counts   []json.Number `json:"bucketCounts"`
						Positive []json.Number `json:"positiveBucketCounts"`
					} `json:"aggregate"`
				}
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Len(t, result.Timeseries, 2)
				for _, series := range result.Timeseries {
					require.Len(t, series.Datapoints, 1)
					got := series.Datapoints[0].Counts
					if kind == "ExponentialHistogram" {
						got = series.Datapoints[0].Positive
					}
					want := tc.second
					if series.SeriesRef == ids[0] {
						want = tc.first
					}
					require.Equal(t, want, got)
				}
				require.Len(t, result.Aggregate, 1)
				got := result.Aggregate[0].Counts
				if kind == "ExponentialHistogram" {
					got = result.Aggregate[0].Positive
				}
				want := make([]json.Number, len(tc.all))
				for i, value := range tc.all {
					want[i] = json.Number(value)
				}
				require.Equal(t, want, got)
			})
		}
	}
}
