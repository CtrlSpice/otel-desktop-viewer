package queries_test

import (
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestHomeNameSortUsesExactCopySpelling(t *testing.T) {
	db := freshDB(t)
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"empty", nil, "null"},
		{"string", "unquoted\n雪😀\u2028", "unquoted\n雪😀\u2028"},
		{"bytes", []byte{0, 1, 255}, "AAH/"},
		{"large integer", int64(9007199254740993), "9007199254740993"},
		{"negative zero", math.Copysign(0, -1), "-0"},
		{"nan", math.NaN(), `"NaN"`},
		{"infinity", math.Inf(1), `"Infinity"`},
		{"negative infinity", math.Inf(-1), `"-Infinity"`},
		{"small decimal", 1e-6, "0.000001"},
		{"small exponent", 1e-7, "1e-7"},
		{"large decimal", 1e20, "100000000000000000000"},
		{"large exponent", 1e21, "1e+21"},
		{"array", []any{int64(9007199254740993), math.Copysign(0, -1), []byte{0, 255}}, "[\n  9007199254740993,\n  -0,\n  \"AP8=\"\n]"},
		{"nested", map[string]any{"<>&\v": []any{"quoted\"\\\v", nil}}, "{\n  \"<>&\\u000b\": [\n    \"quoted\\\"\\\\\\u000b\",\n    null\n  ]\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := pcommon.NewValueEmpty()
			require.NoError(t, value.FromRaw(tc.value))
			encoded, err := ingest.ValueBytes(value)
			require.NoError(t, err)
			var text string
			err = db.QueryRowContext(t.Context(), `WITH native AS (
                SELECT variant_to_parquet_variant(variant_bytes_to_variant($1::BLOB))::struct(metadata BLOB, value BLOB) AS parts)
                SELECT otel_home_value_text(parts.metadata, parts.value) FROM native`, encoded).Scan(&text)
			require.NoError(t, err)
			require.Equal(t, tc.want, text)
		})
	}
}
