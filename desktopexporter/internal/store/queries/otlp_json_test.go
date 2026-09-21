package queries_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOTLPAnyValue(t *testing.T) {
	db := macroDB(t)
	encoded := `{"kind":"map","value":[` +
		`{"key":"MiXeD_snake\n\"é","value":{"kind":"array","value":[` +
		`{"kind":"empty","value":null},` +
		`{"kind":"int64","value":"-9223372036854775808"},` +
		`{"kind":"int64","value":"9223372036854775807"},` +
		`{"kind":"bool","value":false},` +
		`{"kind":"double","value":"0x8000000000000000"},` +
		`{"kind":"double","value":1.25},` +
		`{"kind":"double","value":"0x7ff8000000000001"},` +
		`{"kind":"double","value":"0x7ff0000000000000"},` +
		`{"kind":"double","value":"0xfff0000000000000"},` +
		`{"kind":"bytes","value":"+/8="}]}}` +
		`]}`

	var document string
	require.NoError(t, db.QueryRow(
		`select otlp_document_text(value) from otlp_any_values([?::json])`, encoded,
	).Scan(&document))
	assert.Equal(t,
		`{"kvlistValue":{"values":[{"key":"MiXeD_snake\n\"é","value":{"arrayValue":{"values":[{},`+
			`{"intValue":"-9223372036854775808"},{"intValue":"9223372036854775807"},{"boolValue":false},`+
			`{"doubleValue":-0.0},{"doubleValue":1.25},{"doubleValue":"NaN"},`+
			`{"doubleValue":"Infinity"},{"doubleValue":"-Infinity"},{"bytesValue":"+/8="}]}}}]}}`,
		document)

	var decoded any
	require.NoError(t, json.Unmarshal([]byte(document), &decoded))
}

func BenchmarkOTLPAnyValue(b *testing.B) {
	db := sharedDB
	var entries strings.Builder
	for i := 0; i < 100; i++ {
		if i > 0 {
			entries.WriteByte(',')
		}
		fmt.Fprintf(&entries, `{"key":"field.%03d","value":{"kind":"array","value":[`, i)
		for j := 0; j < 10; j++ {
			if j > 0 {
				entries.WriteByte(',')
			}
			fmt.Fprintf(&entries, `{"kind":"int64","value":"%d"}`, i*10+j)
		}
		entries.WriteString(`]}}`)
	}
	encoded := `{"kind":"map","value":[` + entries.String() + `]}`

	for _, threads := range []int{1, 4} {
		b.Run(fmt.Sprintf("threads-%d", threads), func(b *testing.B) {
			_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
			require.NoError(b, err)
			for i := 0; i < b.N; i++ {
				var document string
				if err := db.QueryRow(`select otlp_document_text(value) from otlp_any_values([?::json])`, encoded).Scan(&document); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkOTLPAnyValueDeep(b *testing.B) {
	db := sharedDB
	for _, threads := range []int{1, 4} {
		for _, depth := range []int{25, 100, 400, 1600} {
			encoded := `{"kind":"int64","value":"1"}`
			for range depth {
				encoded = `{"kind":"array","value":[` + encoded + `]}`
			}
			b.Run(fmt.Sprintf("threads-%d/depth-%d", threads, depth), func(b *testing.B) {
				_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
				require.NoError(b, err)
				for i := 0; i < b.N; i++ {
					var document string
					if err := db.QueryRow(`select otlp_document_text(value) from otlp_any_values([?::json])`, encoded).Scan(&document); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkOTLPAnyValueBatch(b *testing.B) {
	db := sharedDB
	values := make([]string, 1000)
	for i := range values {
		children := make([]string, 8)
		for j := range children {
			children[j] = fmt.Sprintf(`{"kind":"int64","value":"%d"}`, i*8+j)
		}
		values[i] = `{"kind":"map","value":[{"key":"values","value":{"kind":"array","value":[` + strings.Join(children, ",") + `]}}]}`
	}
	encoded := `{"kind":"array","value":[` + strings.Join(values, ",") + `]}`
	for _, threads := range []int{1, 4} {
		b.Run(fmt.Sprintf("threads-%d", threads), func(b *testing.B) {
			_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
			require.NoError(b, err)
			for i := 0; i < b.N; i++ {
				var document string
				require.NoError(b, db.QueryRow(`
					select string_agg(value::varchar, '' order by source_id)
					from otlp_any_values(json_extract(?::json, '$.value[*]'))`, encoded,
				).Scan(&document))
			}
		})
	}
}

func TestOTLPAnyValueDeep(t *testing.T) {
	db := macroDB(t)
	encoded := `{"kind":"int64","value":"1"}`
	expected := `{"intValue":"1"}`
	for range 200 {
		encoded = `{"kind":"array","value":[` + encoded + `]}`
		expected = `{"arrayValue":{"values":[` + expected + `]}}`
	}
	var document string
	require.NoError(t, db.QueryRow(
		`select otlp_document_text(value) from otlp_any_values([?::json])`, encoded,
	).Scan(&document))
	assert.Equal(t, expected, document)
}

func TestOTLPAnyValueKeepsOrderingAndSourceCorrelation(t *testing.T) {
	db := macroDB(t)
	inputs := []string{
		`{"kind":"array","value":[{"kind":"empty","value":null},{"kind":"map","value":[]},{"kind":"array","value":[]},{"kind":"string","value":"one"}]}`,
		`{"kind":"map","value":[{"key":"kind","value":{"kind":"int64","value":"0"}},{"key":"value","value":{"kind":"string","value":"line\n\\\"é"}}]}`,
	}
	for i := 0; i < 12; i++ {
		inputs[0] = strings.TrimSuffix(inputs[0], `]}`) + fmt.Sprintf(`,{"kind":"int64","value":"%d"}]}`, i)
	}

	var document string
	require.NoError(t, db.QueryRow(`
		with batch as materialized (
			select json_object('kind', 'array', 'value', list(encoded order by source_id)) as encoded
			from (values (2, ?::json), (1, ?::json)) source(source_id, encoded)
		)
		select otlp_document_text(json_object(
			'arrayValue', json_object('values', list(value order by source_id))))
		from otlp_any_values((select json_extract(encoded, '$.value[*]') from batch))`, inputs[1], inputs[0]).Scan(&document))
	assert.Contains(t, document, `{"arrayValue":{"values":[{"arrayValue":{"values":[{},{"kvlistValue":{"values":[]}},{"arrayValue":{"values":[]}},{"stringValue":"one"},{"intValue":"0"}`)
	assert.Contains(t, document, `{"intValue":"11"}]}},{"kvlistValue"`)
	assert.Contains(t, document, `{"key":"value","value":{"stringValue":"line\n\\\"é"}}]}}]}}`)
}

func TestOTLPAnyValueEmptyContainers(t *testing.T) {
	db := macroDB(t)
	for name, tc := range map[string][2]string{
		"empty value": {`{"kind":"empty","value":null}`, `{}`},
		"empty array": {`{"kind":"array","value":[]}`, `{"arrayValue":{"values":[]}}`},
		"empty map":   {`{"kind":"map","value":[]}`, `{"kvlistValue":{"values":[]}}`},
	} {
		t.Run(name, func(t *testing.T) {
			var got string
			require.NoError(t, db.QueryRow(
				`select otlp_document_text(value) from otlp_any_values([?::json])`, tc[0],
			).Scan(&got))
			assert.Equal(t, tc[1], got)
		})
	}
}

func TestOTLPConversionRejectsNullAndMalformedValues(t *testing.T) {
	db := macroDB(t)
	for name, query := range map[string]string{
		"SQL null field":        `select otlp_document_text(json_object('value', null))`,
		"SQL null list element": `select otlp_document_text(json_object('values', [json('{}'), null::json]))`,
		"SQL null batch":        `select otlp_document_text(value) from otlp_any_values(null::json[])`,
		"SQL null value":        `select otlp_document_text(value) from otlp_any_values([null::json])`,
		"unknown stored kind":   `select otlp_document_text(value) from otlp_any_values(['{"kind":"future","value":1}'::json])`,
		"missing nested kind":   `select otlp_document_text(value) from otlp_any_values(['{"kind":"array","value":[{"value":"lost"}]}'::json])`,
		"missing map key":       `select otlp_document_text(value) from otlp_any_values(['{"kind":"map","value":[{"value":{"kind":"empty","value":null}}]}'::json])`,
		"non-array container":   `select otlp_document_text(value) from otlp_any_values(['{"kind":"array","value":{}}'::json])`,
	} {
		t.Run(name, func(t *testing.T) {
			var got sql.NullString
			err := db.QueryRow(query).Scan(&got)
			require.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), "OTLP document contains SQL NULL") ||
					strings.Contains(err.Error(), "stored OTel value batch is SQL NULL") ||
					strings.Contains(err.Error(), "stored OTel value is SQL NULL") ||
					strings.Contains(err.Error(), "unknown stored OTel value kind") ||
					strings.Contains(err.Error(), "stored OTel value contains a disconnected node") ||
					strings.Contains(err.Error(), "stored OTel container"),
				err.Error())
		})
	}
}
