package queries_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

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
