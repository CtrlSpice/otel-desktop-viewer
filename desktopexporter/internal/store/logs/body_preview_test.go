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
