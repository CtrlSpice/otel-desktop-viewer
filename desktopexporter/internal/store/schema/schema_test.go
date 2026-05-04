package schema_test

import (
	"database/sql"
	"fmt"
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

func TestMacros_FloorDiv(t *testing.T) {
	db := setupMacroDB(t)

	// The whole point of this macro vs. SQL's `/` is correct rounding for
	// negative numerators. Cases cover:
	//   - positive / positive (matches truncation, sanity check)
	//   - positive / positive with remainder (matches truncation)
	//   - exact division (no remainder, sign doesn't matter)
	//   - negative / positive with remainder (DIVERGES from truncation:
	//     floor goes toward -inf, trunc toward 0)
	//   - negative / positive evenly (no divergence)
	//   - zero numerator (always 0)
	cases := []struct {
		name  string
		query string
		want  int64
	}{
		{"positive evenly divides", "select floor_div(6, 2)", 3},
		{"positive with remainder", "select floor_div(7, 2)", 3},
		{"negative evenly divides", "select floor_div(-6, 2)", -3},
		{"negative with remainder rounds toward -inf", "select floor_div(-7, 2)", -4},
		{"single negative bucket downscales correctly", "select floor_div(-1, 2)", -1},
		{"zero numerator", "select floor_div(0, 4)", 0},
		{"large factor on negative offset", "select floor_div(-160, 16)", -10},
		{"large factor on negative offset with remainder", "select floor_div(-161, 16)", -11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v sql.NullInt64
			require.NoErrorf(t, db.QueryRow(tc.query).Scan(&v), "query failed: %s", tc.query)
			require.True(t, v.Valid, "result should not be NULL")
			assert.Equal(t, tc.want, v.Int64)
		})
	}
}

func TestMacros_DownscaleExpBuckets(t *testing.T) {
	db := setupMacroDB(t)

	// Each case names the input and the expected output struct, then we probe
	// individual fields (offset, len(counts), and specific counts entries) via
	// dot/index access in SQL. This avoids scanning struct/list values
	// directly through database/sql, same trick the bucket-shape tests use.
	type probe struct {
		name  string
		query string
		want  int64
	}
	cases := []struct {
		name   string
		setup  string // common select expression for downscaling, used as a CTE-like alias
		probes []probe
	}{
		{
			name:  "levels=0 is identity",
			setup: "downscale_exp_buckets([10, 20, 30], 5, 0)",
			probes: []probe{
				{"offset unchanged", "select (%s).offset", 5},
				{"length unchanged", "select len((%s).counts)", 3},
				{"counts[1] unchanged", "select ((%s).counts)[1]", 10},
				{"counts[3] unchanged", "select ((%s).counts)[3]", 30},
			},
		},
		{
			name:  "levels=1 halves resolution at offset 0",
			setup: "downscale_exp_buckets([10, 20, 30, 40], 0, 1)",
			probes: []probe{
				{"new offset = floor_div(0, 2) = 0", "select (%s).offset", 0},
				{"length halves to 2", "select len((%s).counts)", 2},
				{"bucket k=0 sums positions 0-1", "select ((%s).counts)[1]", 30},
				{"bucket k=1 sums positions 2-3", "select ((%s).counts)[2]", 70},
			},
		},
		{
			name:  "levels=1 with positive offset that's odd",
			setup: "downscale_exp_buckets([10, 20, 30], 5, 1)",
			// Original indices 5,6,7. floor_div(5,2)=2 (alone), floor_div(6,2)=3,
			// floor_div(7,2)=3. So output: {offset:2, counts:[10, 50]}.
			probes: []probe{
				{"new offset = 2", "select (%s).offset", 2},
				{"length is 2", "select len((%s).counts)", 2},
				{"k=2 has only original bucket 5", "select ((%s).counts)[1]", 10},
				{"k=3 sums originals 6+7", "select ((%s).counts)[2]", 50},
			},
		},
		{
			name:  "levels=1 with negative offset",
			setup: "downscale_exp_buckets([10, 20, 30], -3, 1)",
			// Original indices -3,-2,-1. floor_div(-3,2)=-2, floor_div(-2,2)=-1,
			// floor_div(-1,2)=-1. So output: {offset:-2, counts:[10, 50]}.
			probes: []probe{
				{"new offset = -2", "select (%s).offset", -2},
				{"length is 2", "select len((%s).counts)", 2},
				{"k=-2 has only original -3", "select ((%s).counts)[1]", 10},
				{"k=-1 sums originals -2 + -1", "select ((%s).counts)[2]", 50},
			},
		},
		{
			name:  "levels=2 quarters resolution",
			setup: "downscale_exp_buckets([1, 1, 1, 1, 1, 1, 1, 1], 0, 2)",
			probes: []probe{
				{"new offset = 0", "select (%s).offset", 0},
				{"length is 2 (8 / 4)", "select len((%s).counts)", 2},
				{"k=0 sums first 4 ones", "select ((%s).counts)[1]", 4},
				{"k=1 sums last 4 ones", "select ((%s).counts)[2]", 4},
			},
		},
		{
			name:  "single bucket downscales to single bucket",
			setup: "downscale_exp_buckets([42], 5, 1)",
			// floor_div(5,2)=2 = floor_div(5,2), so one output bucket.
			probes: []probe{
				{"offset = 2", "select (%s).offset", 2},
				{"length is 1", "select len((%s).counts)", 1},
				{"value preserved", "select ((%s).counts)[1]", 42},
			},
		},
	}
	for _, tc := range cases {
		for _, p := range tc.probes {
			t.Run(tc.name+"/"+p.name, func(t *testing.T) {
				query := fmt.Sprintf(p.query, tc.setup)
				var v sql.NullInt64
				require.NoErrorf(t, db.QueryRow(query).Scan(&v), "query failed: %s", query)
				require.True(t, v.Valid, "result should not be NULL: %s", query)
				assert.Equal(t, p.want, v.Int64)
			})
		}
	}

	// Mass conservation: downscaling can never change the total count, only
	// how it's distributed across buckets. Worth a separate sanity assertion
	// because it catches whole classes of off-by-one errors at once.
	t.Run("mass is conserved across levels", func(t *testing.T) {
		got, ok := scalarFloat(t, db, `
			select list_sum((downscale_exp_buckets([3, 7, 11, 13, 17, 19, 23, 29], -2, 2)).counts)::double
		`)
		require.True(t, ok)
		assert.InDelta(t, 122.0, got, 0)
	})

	// Compose with sum_bucket_vectors: downscale stream A from scale 1 to
	// scale 0, then merge with stream B (already at scale 0). Verify the
	// merged bucket counts. We deliberately stop short of feeding the result
	// into exp_hist_quantile here because DuckDB's type inference through
	// the deeply-nested macro chain (exp_hist_quantile -> exp_buckets ->
	// exp_pos_buckets) gets confused about whether the merged counts are
	// BIGINT[] or BIGINT[][] when the downscale output flows through a CTE
	// column, and the workaround would obscure what's being tested.
	// TestMacros_SumBucketVectors already covers sum_bucket_vectors -> hist_quantile,
	// and the per-bucket assertions above prove downscale's correctness.
	t.Run("composes with sum_bucket_vectors", func(t *testing.T) {
		// Stream A at scale 1, offset 0, counts [10, 20, 30, 40] (4 buckets).
		// Stream B at scale 0, offset 0, counts [15, 35] (2 buckets).
		// Downscale A -> [30, 70] at scale 0. Merged with B = [45, 105].
		query := `
			with downscaled as (
				select (downscale_exp_buckets([10, 20, 30, 40], 0, 1)).counts as c
			)
			select
				(sum_bucket_vectors([c, [15, 35]]))[1] as merged_0,
				(sum_bucket_vectors([c, [15, 35]]))[2] as merged_1
			from downscaled
		`
		var b0, b1 sql.NullInt64
		require.NoError(t, db.QueryRow(query).Scan(&b0, &b1))
		require.True(t, b0.Valid && b1.Valid)
		assert.Equal(t, int64(45), b0.Int64)
		assert.Equal(t, int64(105), b1.Int64)
	})
}

func TestMacros_FoldBelowCutoff(t *testing.T) {
	db := setupMacroDB(t)

	// Each subtest probes one field of the returned struct against an
	// expected int64. The triple {counts, offset, folded} fully describes
	// the macro's contract, so per-field assertions catch shape and
	// arithmetic regressions cheaply.
	cases := []struct {
		name  string
		query string
		want  int64
	}{
		// cutoff NULL: pure no-op. counts unchanged, offset unchanged,
		// folded = 0. Common case in production (every stream has
		// zero_threshold = 0, so target_zero_threshold = 0 and the cutoff
		// computation short-circuits to NULL).
		{
			"null cutoff: length unchanged",
			"select len((fold_below_cutoff([10, 20, 30], 5, cast(null as bigint))).counts)",
			3,
		},
		{
			"null cutoff: offset unchanged",
			"select (fold_below_cutoff([10, 20, 30], 5, cast(null as bigint))).offset",
			5,
		},
		{
			"null cutoff: nothing folded",
			"select (fold_below_cutoff([10, 20, 30], 5, cast(null as bigint))).folded",
			0,
		},

		// cutoff < offset_: also a no-op. None of the buckets sit at or
		// below the threshold.
		{
			"cutoff below first bucket: length unchanged",
			"select len((fold_below_cutoff([10, 20, 30], 5, 3)).counts)",
			3,
		},
		{
			"cutoff below first bucket: offset unchanged",
			"select (fold_below_cutoff([10, 20, 30], 5, 3)).offset",
			5,
		},
		{
			"cutoff below first bucket: nothing folded",
			"select (fold_below_cutoff([10, 20, 30], 5, 3)).folded",
			0,
		},

		// cutoff == offset_: fold exactly the first bucket.
		{
			"cutoff at first bucket: length drops by 1",
			"select len((fold_below_cutoff([10, 20, 30], 5, 5)).counts)",
			2,
		},
		{
			"cutoff at first bucket: offset advances by 1",
			"select (fold_below_cutoff([10, 20, 30], 5, 5)).offset",
			6,
		},
		{
			"cutoff at first bucket: folded = first bucket count",
			"select (fold_below_cutoff([10, 20, 30], 5, 5)).folded",
			10,
		},
		{
			"cutoff at first bucket: surviving first count is original second",
			"select ((fold_below_cutoff([10, 20, 30], 5, 5)).counts)[1]",
			20,
		},

		// cutoff in middle: fold first two of three.
		{
			"cutoff folds two: length is 1",
			"select len((fold_below_cutoff([10, 20, 30], 5, 6)).counts)",
			1,
		},
		{
			"cutoff folds two: offset advances by 2",
			"select (fold_below_cutoff([10, 20, 30], 5, 6)).offset",
			7,
		},
		{
			"cutoff folds two: folded sums first two",
			"select (fold_below_cutoff([10, 20, 30], 5, 6)).folded",
			30,
		},
		{
			"cutoff folds two: only third bucket remains",
			"select ((fold_below_cutoff([10, 20, 30], 5, 6)).counts)[1]",
			30,
		},

		// cutoff at the very last bucket: fold everything.
		{
			"cutoff at last bucket: result is empty",
			"select len((fold_below_cutoff([10, 20, 30], 5, 7)).counts)",
			0,
		},
		{
			"cutoff at last bucket: folded is full sum",
			"select (fold_below_cutoff([10, 20, 30], 5, 7)).folded",
			60,
		},

		// cutoff way past the array: still folds everything cleanly,
		// no nonsense slice indices.
		{
			"cutoff well above range: result is empty",
			"select len((fold_below_cutoff([10, 20, 30], 5, 999)).counts)",
			0,
		},
		{
			"cutoff well above range: folded is full sum",
			"select (fold_below_cutoff([10, 20, 30], 5, 999)).folded",
			60,
		},

		// Negative offsets work the same -- macro is sign-agnostic over
		// integer comparison. (In practice the negative-bucket pipeline
		// would compute its own cutoff symmetrically, but the macro itself
		// doesn't care which side it's used on.)
		{
			"negative offset, cutoff folds first: length",
			"select len((fold_below_cutoff([10, 20, 30], -5, -4)).counts)",
			1,
		},
		{
			"negative offset, cutoff folds first: folded",
			"select (fold_below_cutoff([10, 20, 30], -5, -4)).folded",
			30,
		},
		{
			"negative offset, cutoff folds first: surviving offset",
			"select (fold_below_cutoff([10, 20, 30], -5, -4)).offset",
			-3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v sql.NullInt64
			require.NoErrorf(t, db.QueryRow(tc.query).Scan(&v), "query failed: %s", tc.query)
			require.True(t, v.Valid, "result should not be NULL")
			assert.Equal(t, tc.want, v.Int64)
		})
	}

	// NULL counts pass through cleanly: counts stays NULL, folded = 0.
	t.Run("null counts pass through with zero folded", func(t *testing.T) {
		var folded sql.NullInt64
		require.NoError(t,
			db.QueryRow("select (fold_below_cutoff(cast(null as bigint[]), 5, 10)).folded").Scan(&folded))
		require.True(t, folded.Valid)
		assert.Equal(t, int64(0), folded.Int64)
	})

	// Mass conservation: folded + sum(remaining counts) must equal the
	// original total, regardless of where the cutoff lands. Catches whole
	// classes of off-by-one errors in slice ranges in one assertion.
	t.Run("mass is conserved across fold", func(t *testing.T) {
		got, ok := scalarFloat(t, db, `
			with r as (select fold_below_cutoff([3, 7, 11, 13, 17, 19], 0, 2) as f)
			select (cast(f.folded as double) + cast(coalesce(list_sum(f.counts), 0) as double))
			from r
		`)
		require.True(t, ok)
		assert.InDelta(t, 70.0, got, 0)
	})
}

func TestMacros_PadLeftToOffset(t *testing.T) {
	db := setupMacroDB(t)

	// Each subtest probes a single position of the result via [] indexing,
	// matching the BucketBuilderShapes / DownscaleExpBuckets style.
	cases := []struct {
		name  string
		query string
		want  int64
	}{
		// target_offset == current_offset: no padding.
		{
			"no padding when offsets equal: length unchanged",
			"select len(pad_left_to_offset([10, 20, 30], 5, 5))",
			3,
		},
		{
			"no padding when offsets equal: first element unchanged",
			"select (pad_left_to_offset([10, 20, 30], 5, 5))[1]",
			10,
		},

		// Pad by 2 (current=5, target=3).
		{
			"pad by 2: length grows by 2",
			"select len(pad_left_to_offset([10, 20, 30], 5, 3))",
			5,
		},
		{
			"pad by 2: first padded slot is zero",
			"select (pad_left_to_offset([10, 20, 30], 5, 3))[1]",
			0,
		},
		{
			"pad by 2: second padded slot is zero",
			"select (pad_left_to_offset([10, 20, 30], 5, 3))[2]",
			0,
		},
		{
			"pad by 2: original first element shifts to position 3",
			"select (pad_left_to_offset([10, 20, 30], 5, 3))[3]",
			10,
		},
		{
			"pad by 2: original last element shifts to position 5",
			"select (pad_left_to_offset([10, 20, 30], 5, 3))[5]",
			30,
		},

		// Negative offsets (real exp-histogram negative-bucket case).
		// current=-3, target=-5 -> pad by 2.
		{
			"negative offsets pad correctly: length",
			"select len(pad_left_to_offset([10, 20], -3, -5))",
			4,
		},
		{
			"negative offsets pad correctly: padding zero",
			"select (pad_left_to_offset([10, 20], -3, -5))[1]",
			0,
		},
		{
			"negative offsets pad correctly: original first shifts",
			"select (pad_left_to_offset([10, 20], -3, -5))[3]",
			10,
		},

		// Caller-invariant violation: target > current. Macro is defensive
		// and returns the input unchanged rather than producing nonsense or
		// failing -- aligned with how downscale_exp_buckets handles negative
		// `levels`.
		{
			"target > current is no-op: length",
			"select len(pad_left_to_offset([10, 20, 30], 3, 5))",
			3,
		},
		{
			"target > current is no-op: first element unchanged",
			"select (pad_left_to_offset([10, 20, 30], 3, 5))[1]",
			10,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v sql.NullInt64
			require.NoErrorf(t, db.QueryRow(tc.query).Scan(&v), "query failed: %s", tc.query)
			require.True(t, v.Valid, "result should not be NULL")
			assert.Equal(t, tc.want, v.Int64)
		})
	}

	// NULL counts pass through. Indexing into NULL list yields NULL,
	// which scans as !v.Valid.
	t.Run("null counts pass through", func(t *testing.T) {
		var v sql.NullInt64
		require.NoError(t,
			db.QueryRow("select (pad_left_to_offset(cast(null as bigint[]), 5, 3))[1]").Scan(&v))
		assert.False(t, v.Valid)
	})

	// Mass conservation across pad: total count never changes, padding only
	// adds zeros. Quick sanity catch for the dual of downscale's mass test.
	t.Run("mass is preserved across padding", func(t *testing.T) {
		got, ok := scalarFloat(t, db, `
			select list_sum(pad_left_to_offset([3, 7, 11, 13], 10, 6))::double
		`)
		require.True(t, ok)
		assert.InDelta(t, 34.0, got, 0)
	})
}

func TestMacros_SumBucketVectors(t *testing.T) {
	db := setupMacroDB(t)

	// Probe individual elements of the returned list via [] indexing -- same
	// pattern TestMacros_BucketBuilderShapes uses to avoid the complexity of
	// scanning a DuckDB list directly through database/sql.
	intCases := []struct {
		name  string
		query string
		want  int64
	}{
		{
			name:  "three equal-length vectors sum element-wise: index 1",
			query: "select (sum_bucket_vectors([[1, 2, 3], [4, 5, 6], [7, 8, 9]]))[1]",
			want:  12,
		},
		{
			name:  "three equal-length vectors sum element-wise: index 2",
			query: "select (sum_bucket_vectors([[1, 2, 3], [4, 5, 6], [7, 8, 9]]))[2]",
			want:  15,
		},
		{
			name:  "three equal-length vectors sum element-wise: index 3",
			query: "select (sum_bucket_vectors([[1, 2, 3], [4, 5, 6], [7, 8, 9]]))[3]",
			want:  18,
		},
		{
			name:  "single-vector input is identity: index 1",
			query: "select (sum_bucket_vectors([[10, 20, 30]]))[1]",
			want:  10,
		},
		{
			name:  "single-vector input is identity: index 3",
			query: "select (sum_bucket_vectors([[10, 20, 30]]))[3]",
			want:  30,
		},
		{
			// Mismatched lengths shouldn't normally occur (caller enforces
			// shared bounds) but the macro must not crash on it. list_zip
			// pads the shorter list with NULL; coalesce turns those into 0.
			name:  "mismatched lengths zero-pad to longest",
			query: "select (sum_bucket_vectors([[1, 2, 3], [4, 5]]))[3]",
			want:  3,
		},
	}
	for _, tc := range intCases {
		t.Run(tc.name, func(t *testing.T) {
			var v sql.NullInt64
			require.NoErrorf(t, db.QueryRow(tc.query).Scan(&v), "query failed: %s", tc.query)
			require.True(t, v.Valid, "result should not be NULL")
			assert.Equal(t, tc.want, v.Int64)
		})
	}

	t.Run("empty input list returns NULL", func(t *testing.T) {
		var v sql.NullInt64
		// Indexing into a NULL list yields NULL, which scans as !v.Valid.
		require.NoError(t,
			db.QueryRow("select (sum_bucket_vectors(cast([] as bigint[][])))[1]").Scan(&v))
		assert.False(t, v.Valid, "indexing a NULL result should be NULL")
	})

	// End-to-end: merge two streams that share bounds, then run hist_quantile
	// on the summed bucket vector. Two streams over bounds [1, 2, 5, 10]:
	//   A counts = [0, 50,  50,  0, 0]
	//   B counts = [0, 30,  50, 20, 0]
	//   sum     = [0, 80, 100, 20, 0]  total = 200
	// p50 target = 100. CDF acc = 0, 80, 180, 200, 200. First acc >= 100 is
	// bucket 3 (lo=2, hi=5, cnt=100, acc_prev=80). Linear interp:
	//   2 + (5 - 2) * (100 - 80) / 100 = 2.6
	t.Run("end-to-end with hist_quantile on merged streams", func(t *testing.T) {
		got, ok := scalarFloat(t, db,
			`select hist_quantile(
				[1.0, 2.0, 5.0, 10.0],
				sum_bucket_vectors([[0, 50, 50, 0, 0], [0, 30, 50, 20, 0]]),
				0.5
			)`)
		require.True(t, ok, "result should not be NULL")
		assert.InDelta(t, 2.6, got, 1e-9)
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
