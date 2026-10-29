package queries

import (
	"bytes"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricNumericSortKeysCompareExactMixedNumbers(t *testing.T) {
	values := []any{int64(math.MinInt64), int64(math.MaxInt64), int64(-9007199254740993), int64(9007199254740993), int64(0), int64(1), float64(1), math.Copysign(0, -1), float64(0), math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, math.MaxFloat64, -math.MaxFloat64, float64(math.MinInt64), float64(math.MaxInt64), math.Nextafter(1, 2)}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		values = append(values, int64(rng.Uint64()))
		v := math.Float64frombits(rng.Uint64())
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			values = append(values, v)
		}
	}
	rational := func(v any) *big.Rat {
		if n, ok := v.(int64); ok {
			return new(big.Rat).SetInt64(n)
		}
		return new(big.Rat).SetFloat64(v.(float64))
	}
	for _, a := range values {
		for _, b := range values {
			require.Equal(t, rational(a).Cmp(rational(b)), bytes.Compare(metricNumericSortKey(a), metricNumericSortKey(b)), "%v vs %v", a, b)
		}
	}
	require.Negative(t, bytes.Compare(metricNumericSortKey(math.Inf(-1)), metricNumericSortKey(-math.MaxFloat64)))
	require.Positive(t, bytes.Compare(metricNumericSortKey(math.Inf(1)), metricNumericSortKey(math.MaxFloat64)))
	require.Positive(t, bytes.Compare(metricNumericSortKey(math.NaN()), metricNumericSortKey(math.Inf(1))))
	require.Nil(t, metricNumericSortKey("10"))
}
