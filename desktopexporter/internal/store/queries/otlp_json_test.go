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
		`select otlp_document_text(otlp_any_value(?::json))`, encoded,
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var document string
		if err := db.QueryRow(`select otlp_document_text(otlp_any_value(?::json))`, encoded).Scan(&document); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOTLPAnyValueDeep(b *testing.B) {
	db := sharedDB
	for _, depth := range []int{25, 50, 100, 200} {
		encoded := `{"kind":"int64","value":"1"}`
		for range depth {
			encoded = `{"kind":"array","value":[` + encoded + `]}`
		}
		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var document string
				if err := db.QueryRow(`select otlp_document_text(otlp_any_value(?::json))`, encoded).Scan(&document); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
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
				`select otlp_document_text(otlp_any_value(?::json))`, tc[0],
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
		"SQL null value":        `select otlp_document_text(otlp_any_value(null::json))`,
		"unknown stored kind":   `select otlp_document_text(otlp_any_value('{"kind":"future","value":1}'::json))`,
	} {
		t.Run(name, func(t *testing.T) {
			var got sql.NullString
			err := db.QueryRow(query).Scan(&got)
			require.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), "OTLP document contains SQL NULL") ||
					strings.Contains(err.Error(), "stored OTel value is SQL NULL") ||
					strings.Contains(err.Error(), "unknown stored OTel value kind"),
				err.Error())
		})
	}
}
