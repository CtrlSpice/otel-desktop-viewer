package queries_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/sqlfunctions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
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
	require.NoError(t, sqlfunctions.Register(t.Context(), db))
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

func TestNativeValueTextMatchesPdata(t *testing.T) {
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
			encoded, err := ingest.ValueBytes(value)
			require.NoError(t, err)
			var text string
			require.NoError(t, macroDB(t).QueryRow("select otel_value_text(variant_bytes_to_variant(?::blob))", encoded).Scan(&text))
			assert.Equal(t, value.AsString(), text)
		})
	}
}

func TestNativeValueTextDoubleFormatting(t *testing.T) {
	// Compare the pinned engine directly with the converter used at ingestion,
	// including values whose shortest decimal form uses an exponent.
	random := rand.New(rand.NewPCG(1, 2))
	for range 128 {
		bits := random.Uint64()
		value := pcommon.NewValueDouble(math.Float64frombits(bits))
		encoded, err := ingest.ValueBytes(value)
		require.NoError(t, err)
		var text string
		require.NoError(t, macroDB(t).QueryRow("select otel_value_text(variant_bytes_to_variant(?::blob))", encoded).Scan(&text))
		assert.Equal(t, value.AsString(), text, fmt.Sprintf("bits %016x", bits))
	}
}

func TestNativeValueTextSelectedDuplicateMapsMatchesPdata(t *testing.T) {
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
		encoded, err := ingest.ValueBytes(value)
		require.NoError(t, err)
		var text string
		require.NoError(t, macroDB(t).QueryRow("select otel_value_text(variant_bytes_to_variant(?::blob))", encoded).Scan(&text))
		assert.Equal(t, value.AsString(), text)
	}
}

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

func nativeOTLP(t *testing.T, v pcommon.Value) string {
	t.Helper()
	encoded, err := ingest.ValueBytes(v)
	require.NoError(t, err)
	var document string
	require.NoError(t, macroDB(t).QueryRow(
		`select otlp_document_text(otlp_value(variant_bytes_to_variant(?)))`, encoded,
	).Scan(&document))
	return document
}

func TestOTLPAnyValue(t *testing.T) {
	v := pcommon.NewValueEmpty()
	values := v.SetEmptyMap().PutEmptySlice("MiXeD_snake\n\"é")
	values.AppendEmpty()
	values.AppendEmpty().SetInt(math.MinInt64)
	values.AppendEmpty().SetInt(math.MaxInt64)
	values.AppendEmpty().SetBool(false)
	values.AppendEmpty().SetDouble(math.Copysign(0, -1))
	values.AppendEmpty().SetDouble(1.25)
	values.AppendEmpty().SetDouble(math.NaN())
	values.AppendEmpty().SetDouble(math.Inf(1))
	values.AppendEmpty().SetDouble(math.Inf(-1))
	values.AppendEmpty().SetEmptyBytes().FromRaw([]byte{0xfb, 0xff})
	document := nativeOTLP(t, v)
	assert.Equal(t,
		`{"kvlistValue":{"values":[{"key":"MiXeD_snake\n\"é","value":{"arrayValue":{"values":[{},`+
			`{"intValue":"-9223372036854775808"},{"intValue":"9223372036854775807"},{"boolValue":false},`+
			`{"doubleValue":-0.0},{"doubleValue":1.25},{"doubleValue":"NaN"},`+
			`{"doubleValue":"Infinity"},{"doubleValue":"-Infinity"},{"bytesValue":"+/8="}]}}}]}}`,
		document)
	var decoded any
	require.NoError(t, json.Unmarshal([]byte(document), &decoded))
}

func TestOTLPAnyValueDeep(t *testing.T) {
	v := pcommon.NewValueEmpty()
	leaf := v
	expected := `{"intValue":"1"}`
	for range 200 {
		leaf = leaf.SetEmptySlice().AppendEmpty()
		expected = `{"arrayValue":{"values":[` + expected + `]}}`
	}
	leaf.SetInt(1)
	assert.Equal(t, expected, nativeOTLP(t, v))
}

func TestOTLPAnyValueKeepsOrderingAndSourceCorrelation(t *testing.T) {
	db := freshDB(t)
	_, err := db.Exec(`create table native_values (id integer, value variant)`)
	require.NoError(t, err)
	values := pcommon.NewValueEmpty()
	s := values.SetEmptySlice()
	s.AppendEmpty()
	s.AppendEmpty().SetEmptyMap()
	s.AppendEmpty().SetEmptySlice()
	s.AppendEmpty().SetStr("one")
	for i := range 12 {
		s.AppendEmpty().SetInt(int64(i))
	}
	m := pcommon.NewValueEmpty()
	m.SetEmptyMap().PutStr("value", "line\n\\\"é")
	m.Map().PutInt("kind", 0)
	for i, v := range []pcommon.Value{m, values} {
		encoded, err := ingest.ValueBytes(v)
		require.NoError(t, err)
		_, err = db.Exec(`insert into native_values values (?, variant_bytes_to_variant(?))`, 2-i, encoded)
		require.NoError(t, err)
	}
	var document string
	require.NoError(t, db.QueryRow(`select otlp_document_text(json_object(
		'arrayValue', json_object('values', list(otlp_value(value) order by id)))) from native_values`).Scan(&document))
	assert.Contains(t, document, `{"arrayValue":{"values":[{"arrayValue":{"values":[{},{"kvlistValue":{"values":[]}},{"arrayValue":{"values":[]}},{"stringValue":"one"},{"intValue":"0"}`)
	assert.Contains(t, document, `{"intValue":"11"}]}},{"kvlistValue"`)
	assert.Contains(t, document, `{"key":"value","value":{"stringValue":"line\n\\\"é"}}]}}]}}`)
}

func TestOTLPAnyValueEmptyContainers(t *testing.T) {
	for name, tc := range map[string]struct {
		set  func(pcommon.Value)
		want string
	}{
		"empty value":  {func(pcommon.Value) {}, `{}`},
		"empty array":  {func(v pcommon.Value) { v.SetEmptySlice() }, `{"arrayValue":{"values":[]}}`},
		"empty map":    {func(v pcommon.Value) { v.SetEmptyMap() }, `{"kvlistValue":{"values":[]}}`},
		"empty bytes":  {func(v pcommon.Value) { v.SetEmptyBytes() }, `{"bytesValue":""}`},
		"empty string": {func(v pcommon.Value) { v.SetStr("") }, `{"stringValue":""}`},
	} {
		t.Run(name, func(t *testing.T) {
			v := pcommon.NewValueEmpty()
			tc.set(v)
			assert.Equal(t, tc.want, nativeOTLP(t, v))
		})
	}
}

func TestOTLPConversionRejectsNullAndNonMapAttributes(t *testing.T) {
	for name, query := range map[string]string{
		"SQL null field":        `select otlp_document_text(json_object('value', null))`,
		"SQL null list element": `select otlp_document_text(json_object('values', [json('{}'), null::json]))`,
		"SQL null value":        `select otlp_value(null::variant)::varchar`,
		"non-map attributes":    `select otlp_attributes(42::variant)::varchar`,
	} {
		t.Run(name, func(t *testing.T) {
			var got sql.NullString
			err := macroDB(t).QueryRow(query).Scan(&got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "OTLP")
		})
	}
}

func BenchmarkOTLPAnyValue(b *testing.B) {
	v := pcommon.NewValueEmpty()
	m := v.SetEmptyMap()
	for i := range 100 {
		s := m.PutEmptySlice(fmt.Sprintf("field.%03d", i))
		for j := range 10 {
			s.AppendEmpty().SetInt(int64(i*10 + j))
		}
	}
	benchmarkNativeOTLP(b, v)
}

func BenchmarkOTLPAnyValueDeep(b *testing.B) {
	for _, depth := range []int{25, 100, 400, 1600} {
		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {
			v := pcommon.NewValueEmpty()
			leaf := v
			for range depth {
				leaf = leaf.SetEmptySlice().AppendEmpty()
			}
			leaf.SetInt(1)
			benchmarkNativeOTLP(b, v)
		})
	}
}

func benchmarkNativeOTLP(b *testing.B, v pcommon.Value) {
	b.Helper()
	encoded, err := ingest.ValueBytes(v)
	require.NoError(b, err)
	b.ResetTimer()
	for range b.N {
		var document string
		require.NoError(b, sharedDB.QueryRow(`select otlp_value(variant_bytes_to_variant(?))::varchar`, encoded).Scan(&document))
		if strings.TrimSpace(document) == "" {
			b.Fatal("missing OTLP value")
		}
	}
}

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
			err = db.QueryRowContext(t.Context(), `with native as (
                select variant_to_parquet_variant(variant_bytes_to_variant($1::blob))::struct(metadata blob, value blob) as parts)
                select otel_home_value_text(parts.metadata, parts.value) from native`, encoded).Scan(&text)
			require.NoError(t, err)
			require.Equal(t, tc.want, text)
		})
	}
}

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
			`'{"z":1,"a":{"z":2,"a":3},"b":[{"z":4,"a":5}]}'::json::variant`,
			map[string]any{"z": int64(1), "a": map[string]any{"z": int64(2), "a": int64(3)}, "b": []any{map[string]any{"z": int64(4), "a": int64(5)}}},
		},
		{
			`{'z': -0.0::double, 'a': 9223372036854775807::bigint, 'bytes': '\x00\xFF'::blob,
			  'nested': [{'z': 'NaN'::double, 'a': '-Infinity'::double}]}::variant`,
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
	require.NoError(t, sqlfunctions.Register(t.Context(), db))
	_, err = db.Exec("set variant_minimum_shredding_size = 0")
	require.NoError(t, err)
	check("reopen")
}
