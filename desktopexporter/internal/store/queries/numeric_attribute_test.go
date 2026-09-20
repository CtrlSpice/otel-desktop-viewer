package queries_test

import (
	"database/sql"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedAttributeNumericDecoding(t *testing.T) {
	db := macroDB(t)

	var version string
	require.NoError(t, db.QueryRow(`select version()`).Scan(&version))
	assert.Equal(t, "v1.5.5", version)

	var textOrder, typedOrder string
	require.NoError(t, db.QueryRow(`
		with attrs(label, value) as (values
			('two', '{"kind":"int64","value":"2"}'::json),
			('ten', '{"kind":"int64","value":"10"}'::json)
		)
		select
			string_agg(label, ',' order by json_extract_string(value, '$.value')),
			string_agg(label, ',' order by attribute_int64(value))
		from attrs
	`).Scan(&textOrder, &typedOrder))
	assert.Equal(t, "ten,two", textOrder)
	assert.Equal(t, "two,ten", typedOrder)

	var min, largeEven, largeOdd, max int64
	require.NoError(t, db.QueryRow(`select
		attribute_int64('{"kind":"int64","value":"-9223372036854775808"}'::json),
		attribute_int64('{"kind":"int64","value":"9007199254740992"}'::json),
		attribute_int64('{"kind":"int64","value":"9007199254740993"}'::json),
		attribute_int64('{"kind":"int64","value":"9223372036854775807"}'::json)
	`).Scan(&min, &largeEven, &largeOdd, &max))
	assert.Equal(t, int64(math.MinInt64), min)
	assert.Equal(t, int64(9_007_199_254_740_992), largeEven)
	assert.Equal(t, int64(9_007_199_254_740_993), largeOdd)
	assert.Equal(t, int64(math.MaxInt64), max)

	for _, bits := range []uint64{
		0,
		0x8000000000000000,
		1,
		0x3ff4000000000000,
		0x7ff0000000000000,
		0xfff0000000000000,
		0x7ff8000000000001,
		0xfff8000000000002,
	} {
		value := math.Float64frombits(bits)
		wire := fmt.Sprintf("0x%016x", bits)
		var extracted string
		var reconstructed float64
		require.NoError(t, db.QueryRow(`select
			'0x' || lower(hex((?::double)::bit::blob)),
			(unhex(?)::bit)::double
		`, value, wire[2:]).Scan(&extracted, &reconstructed))
		assert.Equal(t, wire, extracted)
		assert.Equal(t, bits, math.Float64bits(reconstructed))
	}

	var finiteOrder, nanEqual, nanAboveInfinity, signedZeroEqual, in, notIn bool
	require.NoError(t, db.QueryRow(`select
		attribute_double('{"kind":"double","value":1.5}'::json) < attribute_double('{"kind":"double","value":2.5}'::json),
		attribute_double('{"kind":"double","value":"0x7ff8000000000001"}'::json) = attribute_double('{"kind":"double","value":"0xfff8000000000002"}'::json),
		attribute_double('{"kind":"double","value":"0x7ff8000000000001"}'::json) > attribute_double('{"kind":"double","value":"0x7ff0000000000000"}'::json),
		attribute_double('{"kind":"double","value":"0x8000000000000000"}'::json) = 0.0,
		attribute_double('{"kind":"double","value":2.0}'::json) in [1.0, 2.0],
		attribute_double('{"kind":"double","value":2.0}'::json) not in [1.0, 3.0]
	`).Scan(&finiteOrder, &nanEqual, &nanAboveInfinity, &signedZeroEqual, &in, &notIn))
	assert.True(t, finiteOrder)
	assert.True(t, nanEqual)
	assert.True(t, nanAboveInfinity)
	assert.True(t, signedZeroEqual)
	assert.True(t, in)
	assert.True(t, notIn)

	var malformed, wrongKind, missingKind, missingValue sql.NullFloat64
	require.NoError(t, db.QueryRow(`select
		attribute_double('{"kind":"double","value":"not-hex"}'::json),
		attribute_double('{"kind":"string","value":"1.5"}'::json),
		attribute_double('{"value":1.5}'::json),
		attribute_double('{"kind":"double"}'::json)
	`).Scan(&malformed, &wrongKind, &missingKind, &missingValue))
	assert.False(t, malformed.Valid)
	assert.False(t, wrongKind.Valid)
	assert.False(t, missingKind.Valid)
	assert.False(t, missingValue.Valid)

	var explainType, plan string
	require.NoError(t, db.QueryRow(`explain select id from attributes where attribute_int64(value) > 42`).Scan(&explainType, &plan))
	assert.Equal(t, "physical_plan", explainType)
	assert.NotContains(t, plan, "attribute_int64")
	assert.Contains(t, plan, "json_extract_string")
}
