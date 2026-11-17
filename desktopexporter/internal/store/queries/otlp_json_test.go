package queries_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

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
