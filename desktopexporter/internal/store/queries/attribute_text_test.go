package queries_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestAttributeTextMatchesPdata(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{"empty", nil}, {"string", "42"}, {"empty string", ""},
		{"escaped string", "quote\"\\\n<>&\u2028\u2029"},
		{"true", true}, {"false", false},
		{"minimum int64", int64(math.MinInt64)}, {"maximum int64", int64(math.MaxInt64)},
		{"wide int64", int64(9007199254740993)},
		{"zero", float64(0)}, {"negative zero", math.Copysign(0, -1)},
		{"integral double", float64(42)}, {"fractional double", 42.5},
		{"small exponent", 1e-7}, {"small decimal", 1e-6},
		{"large decimal", 1e20}, {"large exponent", 1e21},
		{"minimum double", math.SmallestNonzeroFloat64}, {"maximum double", math.MaxFloat64},
		{"nan", math.NaN()}, {"infinity", math.Inf(1)}, {"negative infinity", math.Inf(-1)},
		{"bytes", []byte{0, 1, 255}}, {"empty bytes", []byte{}},
		{"empty array", []any{}}, {"empty map", map[string]any{}},
		{"nested empty bytes", []any{[]byte{}, map[string]any{"empty": []byte{}}}},
		{"array", []any{nil, "<>&\u2028", true, int64(math.MaxInt64), math.Copysign(0, -1), 1e21, []byte{255}}},
		{"map", map[string]any{"z": int64(math.MinInt64), "a": "value", "<>&\u2029": []any{1e-7, false}}},
		{"nested", []any{map[string]any{"b": []any{1e21, nil}, "a": map[string]any{"x": []byte{1, 2}}}}},
		{"array with nan", []any{math.NaN()}},
		{"map with nested infinity", map[string]any{"a": []any{math.Inf(1)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := pcommon.NewValueEmpty()
			require.NoError(t, value.FromRaw(test.value))
			encoded, err := util.EncodeValue(value)
			require.NoError(t, err)
			var text string
			require.NoError(t, macroDB(t).QueryRow("SELECT attribute_text(?::JSON)", string(encoded)).Scan(&text))
			assert.Equal(t, value.AsString(), text)
		})
	}
}

func TestAttributeTextDoubleFormatting(t *testing.T) {
	// Compare the pinned engine directly with the converter used at ingestion,
	// including values whose shortest decimal form uses an exponent.
	random := rand.New(rand.NewPCG(1, 2))
	for range 128 {
		bits := random.Uint64()
		value := pcommon.NewValueDouble(math.Float64frombits(bits))
		encoded, err := util.EncodeValue(value)
		require.NoError(t, err)
		var text string
		require.NoError(t, macroDB(t).QueryRow("SELECT attribute_text(?::JSON)", string(encoded)).Scan(&text))
		assert.Equal(t, value.AsString(), text, fmt.Sprintf("bits %016x", bits))
	}
}

func TestAttributeTextSelectedDuplicateMapsMatchesPdata(t *testing.T) {
	for _, entries := range []string{
		`{"key":"x","value":{"stringValue":"z"}},{"key":"x","value":{"stringValue":"a"}}`,
		`{"key":"x","value":{"stringValue":"a"}},{"key":"x","value":{"stringValue":"z"}}`,
		`{"key":"x","value":{"intValue":"9223372036854775807"}},{"key":"x","value":{"stringValue":"last"}}`,
		`{"key":"x","value":{"doubleValue":"NaN"}},{"key":"x","value":{"intValue":"42"}}`,
	} {
		logs, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(
			`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"kvlistValue":{"values":[` + entries + `]}}}]}]}]}`))
		require.NoError(t, err)
		value := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body()
		encoded, err := util.EncodeValue(value)
		require.NoError(t, err)
		var text string
		require.NoError(t, macroDB(t).QueryRow("SELECT attribute_text(?::JSON)", string(encoded)).Scan(&text))
		assert.Equal(t, value.AsString(), text)
	}
}
