package queries_test

import (
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
	assert.Equal(t, "v2.0.0", version)

	var textOrder, typedOrder string
	require.NoError(t, db.QueryRow(`
		with attrs(label, value) as (values
			('two', 2::bigint::variant),
			('ten', 10::bigint::variant)
		)
		select
			string_agg(label, ',' order by value::varchar),
			string_agg(label, ',' order by value::bigint)
		from attrs
	`).Scan(&textOrder, &typedOrder))
	assert.Equal(t, "ten,two", textOrder)
	assert.Equal(t, "two,ten", typedOrder)

	var min, largeEven, largeOdd, max int64
	require.NoError(t, db.QueryRow(`select
		(-9223372036854775808)::bigint::variant::bigint,
		9007199254740992::bigint::variant::bigint,
		9007199254740993::bigint::variant::bigint,
		9223372036854775807::bigint::variant::bigint
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
		1.5::double::variant::double < 2.5::double::variant::double,
		(unhex('7ff8000000000001')::bit)::double::variant::double = (unhex('fff8000000000002')::bit)::double::variant::double,
		(unhex('7ff8000000000001')::bit)::double::variant::double > (unhex('7ff0000000000000')::bit)::double::variant::double,
		(unhex('8000000000000000')::bit)::double::variant::double = 0.0,
		2.0::double::variant::double in [1.0, 2.0],
		2.0::double::variant::double not in [1.0, 3.0]
	`).Scan(&finiteOrder, &nanEqual, &nanAboveInfinity, &signedZeroEqual, &in, &notIn))
	assert.True(t, finiteOrder)
	assert.True(t, nanEqual)
	assert.True(t, nanAboveInfinity)
	assert.True(t, signedZeroEqual)
	assert.True(t, in)
	assert.True(t, notIn)

	var explainType, plan string
	require.NoError(t, db.QueryRow(`explain select id from logs where attributes['number']::bigint > 42`).Scan(&explainType, &plan))
	assert.Equal(t, "physical_plan", explainType)
	assert.NotContains(t, plan, "otel_wire_json")
	assert.NotContains(t, plan, "json_extract_string")
}
