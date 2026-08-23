package queries_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// TestListSliceBoundsAreAsymmetric pins DuckDB behaviour this schema depends on
// but does not control.
//
// list_slice clamps an out-of-range *end* and does not clamp an out-of-range
// *start*: a start below 1 yields an empty list. Two macros here compute slice
// bounds arithmetically from an offset that can be negative, so which of those
// two rules applies decides whether counts survive or silently disappear --
// and it is not the kind of difference that shows up in review. A comment in
// fold_below_cutoff asserted the opposite for a while; the code was saved by a
// guard elsewhere rather than by the behaviour it claimed.
//
// This is a characterisation test: it is not asserting that DuckDB is right,
// only recording what it does, so an upgrade that changes it fails here with
// an explanation rather than somewhere downstream as a wrong histogram. If it
// ever fails, read the two macros below before touching this file.
func TestListSliceBoundsAreAsymmetric(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

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
