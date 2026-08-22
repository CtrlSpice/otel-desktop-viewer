package queries_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// TestDownscaleExpBuckets pins what downscaling an exponential histogram
// produces, by value rather than by comparison.
//
// It used to be checked against its own previous implementation, which stopped
// being possible when that implementation was replaced: the old form filtered
// the whole input once per output bucket and was quadratic in bucket count.
// The replacement takes each output bucket's inputs as a contiguous slice,
// which is only correct because position -> bucket is monotonic. These cases
// are worked by hand so the property is pinned to arithmetic and not to
// whichever formulation happens to be in the file.
//
// The clamped ends are the part worth testing. Only the first and last output
// buckets can be partial -- every bucket between them is exactly 2^levels wide
// -- so an off-by-one in the bounds shows up at the edges and nowhere else,
// which is why several cases below start at an offset that does not divide
// evenly.
func TestDownscaleExpBuckets(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL)
	}
	for _, stmt := range queries.Macros() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}

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

// Total count is conserved: downscaling merges buckets, it never invents or
// drops observations. Cheap to assert over many shapes, and it catches a whole
// class of bounds error that hand-written cases can miss -- a slice that skips
// an input or counts one twice changes the sum.
func TestDownscaleConservesTotal(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL)
	}
	for _, stmt := range queries.Macros() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}

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
