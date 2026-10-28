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

func TestNativeAttributeEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery.duckdb")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, install(db))
	_, err = db.Exec(`set variant_minimum_shredding_size = 0;
		create table discovery_values (id integer, value variant);
		-- Independent reference: traverse the established wire representation.
		create macro wire_entries(attrs) as table (
			with recursive nodes(path, value) as (
				select []::json[], otel_value_json(attrs)
				union all
				select list_append(nodes.path,
					case when nodes.value->>'kind' = 'map'
						then to_json(child.value->>'key') else to_json(child.key::bigint) end),
					case when nodes.value->>'kind' = 'map'
						then child.value->'value' else child.value end
				from nodes, json_each(nodes.value, '$.value') child
				where nodes.value->>'kind' in ('map', 'array')
			)
			select path, case when len(path) = 1 then path[1]->>'$'
				else to_json(path)::varchar end as key,
				value, value->>'kind' as type
			from nodes where len(path) > 0
		)`)
	require.NoError(t, err)
	for id, raw := range []any{
		map[string]any{
			"": "empty key", "a.b": true, "a": map[string]any{"b": false},
			"quote\"/雪": []byte{0, 255}, "max": int64(math.MaxInt64),
			"min": int64(math.MinInt64), "zero": math.Copysign(0, -1),
			"null": nil, "nan": math.NaN(), "infinity": math.Inf(1),
			"array": []any{nil, []any{}, map[string]any{}, map[string]any{"0": "index key"}},
		},
		[]any{map[string]any{"nested": []any{int64(42), "42"}}, nil, math.Copysign(0, -1)},
		map[string]any{}, []any{}, nil, "scalar root",
	} {
		value := pcommon.NewValueEmpty()
		require.NoError(t, value.FromRaw(raw))
		encoded, err := ingest.ValueBytes(value)
		require.NoError(t, err)
		_, err = db.Exec(`insert into discovery_values values (?, variant_bytes_to_variant(?))`, id, encoded)
		require.NoError(t, err)
	}
	// Native SQL objects can have unsorted fields, unlike the ingestion builder.
	_, err = db.Exec(`insert into discovery_values values (6,
		{'z': [{'z': -0.0::double, 'a': 9223372036854775807::bigint}],
		 'a': {'z': '\x00\xFF'::blob, 'a': '雪'}}::variant)`)
	require.NoError(t, err)
	readEntries := func(macro string) [][5]string {
		t.Helper()
		rows, err := db.Query(`select id::varchar, to_json(entry.path)::varchar,
			entry.key, entry.type, entry.value::varchar
			from discovery_values, ` + macro + `(value) entry order by id, 2`)
		require.NoError(t, err)
		defer rows.Close()
		var result [][5]string
		for rows.Next() {
			var row [5]string
			require.NoError(t, rows.Scan(&row[0], &row[1], &row[2], &row[3], &row[4]))
			result = append(result, row)
		}
		require.NoError(t, rows.Err())
		return result
	}
	check := func(stage string) {
		t.Helper()
		want := readEntries("wire_entries")
		require.NotEmpty(t, want)
		require.Equal(t, want, readEntries("attribute_entries"), stage)
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

	// Key/type discovery must not call the wire encoder, even through the public
	// wrapper. Container payloads can be large and are only needed on selection.
	_, err = db.Exec(`create or replace macro otel_value_json(value) as
		(error('key discovery serialized a payload')::json)`)
	require.NoError(t, err)
	for _, macro := range []string{"attribute_native_entries", "attribute_entries"} {
		var count int
		err = db.QueryRow(`select count(distinct (entry.path, entry.key, entry.type))
			from discovery_values, ` + macro + `(value) entry`).Scan(&count)
		require.NoError(t, err, macro)
		require.Positive(t, count)
	}
}
