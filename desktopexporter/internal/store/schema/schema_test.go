package schema_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMacroDB stands up a fresh in-memory DuckDB and applies all macro
// creation queries. Tests that only need the macros (no tables/indexes) use
// this rather than store.NewStore to avoid an import cycle (store -> schema).
func setupMacroDB(t *testing.T) *sql.DB {
	t.Helper()

	connector, err := duckdb.NewConnector("", nil)
	require.NoError(t, err, "duckdb connector should open")
	t.Cleanup(func() { _ = connector.Close() })

	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })

	for i, q := range schema.MacroCreationQueries {
		_, err := db.Exec(q)
		require.NoErrorf(t, err, "creating macro %d should succeed", i)
	}
	return db
}

// scalarFloat runs a query that returns one numeric column and returns the
// value as a float64. The bool indicates whether the result was non-NULL.
func scalarFloat(t *testing.T, db *sql.DB, query string) (float64, bool) {
	t.Helper()
	var v sql.NullFloat64
	require.NoErrorf(t, db.QueryRow(query).Scan(&v), "query failed: %s", query)
	return v.Float64, v.Valid
}

func TestMacros_InterpolationKernels(t *testing.T) {
	db := setupMacroDB(t)

	cases := []struct {
		name  string
		query string
		want  float64
	}{
		{
			name:  "linear midpoint of [0,10] at q=0.5",
			query: "select interp_linear(0.0, 10.0, 0, 100, 50)",
			want:  5.0,
		},
		{
			name:  "loglin geometric midpoint of [1,100]",
			query: "select interp_loglin(1.0, 100.0, 0, 100, 50)",
			want:  10.0,
		},
		{
			name:  "loglin fallback to linear when lo=0",
			query: "select interp_loglin(0.0, 10.0, 0, 100, 50)",
			want:  5.0,
		},
		{
			name:  "loglin fallback to linear when hi=0",
			query: "select interp_loglin(-10.0, 0.0, 0, 100, 50)",
			want:  -5.0,
		},
		{
			name:  "loglin fallback to linear on sign mismatch",
			query: "select interp_loglin(-5.0, 5.0, 0, 100, 50)",
			want:  0.0,
		},
		{
			name:  "loglin both negative (no fallback)",
			query: "select interp_loglin(-100.0, -1.0, 0, 100, 50)",
			want:  -10.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scalarFloat(t, db, tc.query)
			require.True(t, ok, "result should not be NULL")
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestMacros_HistogramQuantile(t *testing.T) {
	db := setupMacroDB(t)

	cases := []struct {
		name  string
		query string
		want  float64
	}{
		{
			// counts cumulative: 0, 50, 100, 100, 100. Total=100, p50 target=50.
			// First bucket with acc >= 50 is bucket 2 (1,2]. Linear interp
			// at fraction 1.0 gives 2.0 (the upper bound).
			name:  "p50 lands cleanly on a bucket boundary",
			query: "select hist_quantile([1.0, 2.0, 5.0, 10.0], [0, 50, 50, 0, 0], 0.5)",
			want:  2.0,
		},
		{
			// counts cumulative: 0, 10, 30, 60, 100. p95 target=95.
			// Lands in the unbounded tail (bucket 5), where lo=hi=10.0,
			// so we clamp to the last known bound.
			name:  "p95 in unbounded tail clamps to last known bound",
			query: "select hist_quantile([1.0, 2.0, 5.0, 10.0], [0, 10, 20, 30, 40], 0.95)",
			want:  10.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scalarFloat(t, db, tc.query)
			require.True(t, ok, "result should not be NULL")
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestMacros_ExpHistogramQuantile(t *testing.T) {
	db := setupMacroDB(t)

	cases := []struct {
		name  string
		query string
		want  float64
		delta float64
	}{
		{
			// scale=0 -> base=2. pos_counts=[50,50] at offset=0:
			//   bucket1 (1,2] cnt=50, bucket2 (2,4] cnt=50. Total=100.
			// p50 target=50 -> first bucket at acc>=50 is bucket1.
			// loglin: 1 * (2/1)^(50/50) = 2.0.
			name:  "positive-only p50 (scale=0, two equal buckets)",
			query: "select exp_hist_quantile(0, 0, [], 0, 0, [50, 50], 0.5)",
			want:  2.0,
			delta: 1e-9,
		},
		{
			// All weight in zero bucket. p50 -> zero bucket -> 0.
			name:  "zero-only p50 returns 0",
			query: "select exp_hist_quantile(0, 0, [], 100, 0, [], 0.5)",
			want:  0.0,
			delta: 1e-9,
		},
		{
			// neg=[10,10], zero=20, pos=[10,10] at scale=0. Total=60.
			// CDF acc: 10, 20, 40, 50, 60. p50 target=30 -> first acc>=30
			// is the zero bucket (acc=40). Loglin over [0,0] falls back to
			// linear -> 0.
			name:  "symmetric three-region p50 lands in zero bucket",
			query: "select exp_hist_quantile(0, 0, [10, 10], 20, 0, [10, 10], 0.5)",
			want:  0.0,
			delta: 1e-9,
		},
		{
			// Reference dataset hand-computed in the planning notes.
			// scale=2 -> base = 2^(2^-2) = 2^0.25 ~= 1.189.
			// counts=[1200,3800,4200,2100,720,280,70,22,6,2] at offset=6.
			// Hand calc predicted p95 ~= 6.35.
			name:  "reference dataset p95 matches hand calc",
			query: "select exp_hist_quantile(2, 0, [], 0, 6, [1200, 3800, 4200, 2100, 720, 280, 70, 22, 6, 2], 0.95)",
			want:  6.349604207872798,
			delta: 1e-6,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scalarFloat(t, db, tc.query)
			require.True(t, ok, "result should not be NULL")
			assert.InDelta(t, tc.want, got, tc.delta)
		})
	}
}

func TestMacros_NullSafety(t *testing.T) {
	db := setupMacroDB(t)

	cases := []struct {
		name  string
		query string
	}{
		{
			name:  "hist_quantile on empty bounds returns NULL",
			query: "select hist_quantile(cast([] as double[]), cast([] as integer[]), 0.5)",
		},
		{
			name:  "hist_quantile with NULL bounds returns NULL",
			query: "select hist_quantile(cast(NULL as double[]), [10], 0.5)",
		},
		{
			name:  "exp_hist_quantile with all-zero counts returns NULL",
			query: "select exp_hist_quantile(0, 0, [], 0, 0, [], 0.5)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := scalarFloat(t, db, tc.query)
			assert.False(t, ok, "result should be NULL")
		})
	}
}

func TestMacros_BucketBuilderShapes(t *testing.T) {
	db := setupMacroDB(t)

	// Float bucket bounds. Each row asserts one struct field of one element
	// in the list returned by a builder.
	floatCases := []struct {
		name  string
		query string
		want  float64
	}{
		// hist_buckets clamps the unbounded extreme buckets to the nearest
		// known bound (lo == hi). Inner buckets span (bounds[i-1], bounds[i]].
		{
			name:  "hist_buckets first bucket clamped to bounds[1]",
			query: "select (hist_buckets([1.0, 2.0, 5.0, 10.0], [10, 20, 30, 40, 50]))[1].lo",
			want:  1.0,
		},
		{
			name:  "hist_buckets last bucket clamped to bounds[end]",
			query: "select (hist_buckets([1.0, 2.0, 5.0, 10.0], [10, 20, 30, 40, 50]))[5].hi",
			want:  10.0,
		},
		{
			name:  "hist_buckets inner bucket lower bound",
			query: "select (hist_buckets([1.0, 2.0, 5.0, 10.0], [10, 20, 30, 40, 50]))[3].lo",
			want:  2.0,
		},
		{
			name:  "hist_buckets inner bucket upper bound",
			query: "select (hist_buckets([1.0, 2.0, 5.0, 10.0], [10, 20, 30, 40, 50]))[3].hi",
			want:  5.0,
		},

		// exp_pos_buckets at scale=0 (base=2), offset=0:
		// bucket i covers (2^(i-1), 2^i].
		{
			name:  "exp_pos_buckets[1] = (1,2]",
			query: "select (exp_pos_buckets(0, 0, [10, 20, 30]))[1].hi",
			want:  2.0,
		},
		{
			name:  "exp_pos_buckets[3] = (4,8]",
			query: "select (exp_pos_buckets(0, 0, [10, 20, 30]))[3].lo",
			want:  4.0,
		},

		// exp_neg_buckets walks most-negative first.
		// Source counts=[10,20,30] -> first emitted is the original last
		// bucket (range -8..-4), last emitted is the original first (-2..-1).
		{
			name:  "exp_neg_buckets[1] = [-8,-4) (most negative first)",
			query: "select (exp_neg_buckets(0, 0, [10, 20, 30]))[1].lo",
			want:  -8.0,
		},
		{
			name:  "exp_neg_buckets[1] upper bound is -4",
			query: "select (exp_neg_buckets(0, 0, [10, 20, 30]))[1].hi",
			want:  -4.0,
		},
		{
			name:  "exp_neg_buckets[3] = [-2,-1) (least negative last)",
			query: "select (exp_neg_buckets(0, 0, [10, 20, 30]))[3].hi",
			want:  -1.0,
		},
	}
	for _, tc := range floatCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scalarFloat(t, db, tc.query)
			require.True(t, ok, "result should not be NULL")
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}

	// Confirm counts are reversed in exp_neg_buckets (the count at the
	// most-negative emitted position should be the original last entry).
	t.Run("exp_neg_buckets reverses counts so most-negative first holds last source count", func(t *testing.T) {
		var first, last int64
		require.NoError(t,
			db.QueryRow("select (exp_neg_buckets(0, 0, [10, 20, 30]))[1].cnt").Scan(&first))
		require.NoError(t,
			db.QueryRow("select (exp_neg_buckets(0, 0, [10, 20, 30]))[3].cnt").Scan(&last))
		assert.Equal(t, int64(30), first, "first emitted neg bucket should hold count from source[3]")
		assert.Equal(t, int64(10), last, "last emitted neg bucket should hold count from source[1]")
	})
}

func TestMacros_Idempotent(t *testing.T) {
	// Re-running every macro through CREATE OR REPLACE must succeed without
	// "already exists" errors. This is the protection against the historical
	// DuckDB quirk where some CREATE statements weren't idempotent and had
	// to be tolerated at the bootstrap layer.
	db := setupMacroDB(t)
	for i, q := range schema.MacroCreationQueries {
		_, err := db.Exec(q)
		require.NoErrorf(t, err, "re-running macro %d should succeed", i)
	}
}
