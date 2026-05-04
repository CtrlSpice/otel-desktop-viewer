package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBucketStartTypePrecision verifies that the bucket_start column produced
// by the adaptive-time-bucketing CTE stays as BIGINT and survives a
// ::varchar cast without scientific notation. DuckDB's `/` operator returns
// DOUBLE even for bigint operands; we use `//` (integer division) instead.
func TestBucketStartTypePrecision(t *testing.T) {
	db := setupMacroDB(t)

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
