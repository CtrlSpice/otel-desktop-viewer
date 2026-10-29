package logs_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestBodyPreview(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	cases := []struct{ name, body, want string }{
		{"string", `{"kind":"string","value":"hello\n  world"}`, "hello world"},
		{"json string stays string", `{"kind":"string","value":"{\"message\":\"hello\"}"}`, `{"message":"hello"}`},
		{"map", `{"kind":"map","value":[{"key":"message","value":{"kind":"string","value":"failed"}},{"key":"count","value":{"kind":"int64","value":"9223372036854775807"}},{"key":"details","value":{"kind":"map","value":[]}}]}`, `{message: "failed", count: 9223372036854775807, details: object (0 fields)}`},
		{"empty map", `{"kind":"map","value":[]}`, "{}"},
		{"empty array", `{"kind":"array","value":[]}`, "[]"},
		{"array", `{"kind":"array","value":[{"kind":"int64","value":"9007199254740993"},{"kind":"bool","value":false},{"kind":"double","value":-0.0},{"kind":"string","value":"tail"}]}`, `[9007199254740993, false, -0.0, … (+1 items)]`},
		{"empty", `{"kind":"empty","value":null}`, "empty"},
		{"bytes", `{"kind":"bytes","value":"AAEC"}`, "bytes (base64): AAEC"},
		{"long", `{"kind":"string","value":"` + strings.Repeat("x", 301) + `"}`, strings.Repeat("x", 299) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readStore(s, func(db *sql.DB) (string, error) {
				var result string
				err := db.QueryRowContext(ctx, `select body_preview(?::json)`, tc.body).Scan(&result)
				return result, err
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNativeBodyPreviewMatchesJSON(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	cases := []struct{ name, expression string }{
		{"string", `'hello\n  world'::variant`},
		{"JSON string", `'{"message":"hello"}'::variant`},
		{"Unicode limit", `repeat('雪😀', 151)::variant`},
		{"long string child", `[repeat('雪😀', 30)]::variant`},
		{"JSON quoting", `['<>&"\\雪' || chr(8) || chr(11) || chr(31) || chr(8232) || chr(8233)]::variant`},
		{"boolean", `[true, false]::variant`},
		{"long key", `map([repeat('雪😀', 20)], ['value'])::variant`},
		{"map order and count", `{'z': 'last', 'a': 9007199254740993::bigint, 'c': {'nested': [1,2,3]}, 'b': 'text'}::variant`},
		{"empty map", `map([]::varchar[], []::varchar[])::variant`},
		{"empty array", `[]::variant`},
		{"empty", `NULL::variant`},
		{"empty child", `[NULL::variant, 1::variant]::variant`},
		{"integer", `9223372036854775807::bigint::variant`},
		{"double", `1.5::double::variant`},
		{"negative zero", `'-0'::double::variant`},
		{"nonfinite", `['NaN'::double, 'Infinity'::double, '-Infinity'::double]::variant`},
		{"bytes", `'\x00\x01\x02'::blob::variant`},
		{"long bytes", `repeat('a', 40)::blob::variant`},
		{"array count", `[1,2,3,4,5]::variant`},
		{"nested array", `[[[1,2,3]]]::variant`},
		{"nested map", `{'context': {'messages': [repeat('nested',10000)]}}::variant`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				var native, legacy string
				err := db.QueryRowContext(ctx, `select native_body_preview(body), body_preview(otel_value_json(body)) from (select `+tc.expression+` as body)`).Scan(&native, &legacy)
				require.NoError(t, err)
				require.Equal(t, legacy, native)
				return nil
			}))
		})
	}
}
