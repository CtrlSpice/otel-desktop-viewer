package queries_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateBucketJSONUsesBoundsPresenceForRepresentation(t *testing.T) {
	db := macroDB(t)

	query := func(bounds, counts, scale, zeroCount, posCounts string) map[string]any {
		t.Helper()
		var raw string
		err := db.QueryRow(`select aggregate_bucket_json(
			1::bigint, 0::bigint, 7::ubigint, 3.5::double,
			` + scale + `, 0.0::double, ` + zeroCount + `,
			{'offset': 0::integer, 'counts': ` + posCounts + `, 'folded': 0::bigint},
			{'offset': 0::integer, 'counts': []::ubigint[], 'folded': 0::bigint},
			` + bounds + `, ` + counts + `, null::json
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
}
