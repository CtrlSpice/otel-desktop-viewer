package queries_test

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// DuckDB permits unsorted native objects, but Arrow correctly requires sorted
// object fields in the Parquet VARIANT projection. Exercise every read encoder
// before and after checkpoint/reopen, including nested objects inside arrays.
func TestNativeReadNormalizesSQLObjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native.duckdb")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, install(db))
	_, err = db.Exec(`set variant_minimum_shredding_size = 0;
		create table native_read_values (id integer, value variant, expected variant)`)
	require.NoError(t, err)
	for i, tc := range []struct {
		expr string
		want any
	}{
		{
			`'{"z":1,"a":{"z":2,"a":3},"b":[{"z":4,"a":5}]}'::JSON::VARIANT`,
			map[string]any{"z": int64(1), "a": map[string]any{"z": int64(2), "a": int64(3)}, "b": []any{map[string]any{"z": int64(4), "a": int64(5)}}},
		},
		{
			`{'z': -0.0::DOUBLE, 'a': 9223372036854775807::BIGINT, 'bytes': '\x00\xFF'::BLOB,
			  'nested': [{'z': 'NaN'::DOUBLE, 'a': '-Infinity'::DOUBLE}]}::VARIANT`,
			map[string]any{"z": math.Copysign(0, -1), "a": int64(math.MaxInt64), "bytes": []byte{0, 255}, "nested": []any{map[string]any{"z": math.NaN(), "a": math.Inf(-1)}}},
		},
	} {
		v := pcommon.NewValueEmpty()
		require.NoError(t, v.FromRaw(tc.want))
		encoded, err := ingest.ValueBytes(v)
		require.NoError(t, err)
		_, err = db.Exec(`insert into native_read_values values (?, `+tc.expr+`, variant_bytes_to_variant(?))`, i, encoded)
		require.NoError(t, err)
	}
	check := func(stage string) {
		t.Helper()
		for _, macro := range []string{"otel_value_json", "otel_value_text", "otlp_value"} {
			rows, err := db.Query(`select ` + macro + `(value)::varchar, ` + macro + `(expected)::varchar from native_read_values order by id`)
			require.NoError(t, err, "%s %s", stage, macro)
			count := 0
			for rows.Next() {
				var got, want string
				require.NoError(t, rows.Scan(&got, &want))
				require.Equal(t, want, got, "%s %s row %d", stage, macro, count)
				count++
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			require.Equal(t, 2, count)
		}
	}
	check("live")
	_, err = db.Exec("checkpoint")
	require.NoError(t, err)
	check("checkpoint")
	require.NoError(t, db.Close())
	db, err = sql.Open("duckdb", path)
	require.NoError(t, err)
	require.NoError(t, queries.RegisterFunctions(t.Context(), db))
	_, err = db.Exec("set variant_minimum_shredding_size = 0")
	require.NoError(t, err)
	check("reopen")
}
