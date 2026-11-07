package queries_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// Execute the frontend generator itself against the installed production schema.
// This catches owner-column drift as well as differences in native value equality.
func TestHomeAttributeMatchesSQL(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the frontend SQL generator")
	}
	db := freshDB(t)
	const trace = "00000000-0000-0000-0000-000000000001"
	const metric = "00000000-0000-0000-0000-000000000002"
	const point = "00000000-0000-0000-0000-000000000004"
	const logID = "00000000-0000-0000-0000-000000000005"
	attrs := func(owner string) string {
		attributes := pcommon.NewMap()
		require.NoError(t, attributes.FromRaw(map[string]any{
			"owner": owner, "literal.dot": "literal", "literal": map[string]any{"dot": "nested"},
			"quote'key": "quote", "": "empty key", "maximum": int64(math.MaxInt64),
			"bytes": []byte{0, 1, 255}, "negative_zero": math.Copysign(0, -1),
			"positive_zero": float64(0), "empty": nil, "empty_string": "",
			"empty_array": []any{}, "empty_map": map[string]any{}, "empty_bytes": []byte{},
			"array": []any{int64(math.MaxInt64), math.Copysign(0, -1), nil},
			"map":   map[string]any{"child": "value"}, "escaped": "quote' <>&\u2028\u2029",
		}))
		encoded, err := ingest.AttributeBytes(attributes)
		require.NoError(t, err)
		return fmt.Sprintf("variant_bytes_to_variant(from_hex('%x'))", encoded)
	}
	resource := func(owner string) string {
		return `struct_pack(id := null::uuid, payload_id := null::uuid, schema_url := '', attributes := ` + attrs(owner) + `, dropped_attributes_count := 0)::otel_resource`
	}
	scope := func(owner string) string {
		return `struct_pack(id := null::uuid, name := '', version := '', schema_url := '', attributes := ` + attrs(owner) + `, dropped_attributes_count := 0)::otel_scope`
	}
	execSQL := func(query string) {
		t.Helper()
		_, err := db.Exec(query)
		require.NoError(t, err)
	}
	execSQL(fmt.Sprintf(`insert into spans(trace_id, span_id, start_time, attributes, resource, scope)
		values ('%s', 1, 100, %s, %s, %s), ('%s', 2, 100, '{}'::json::variant, %s, %s),
		('%s', 3, 10, %s, %s, %s)`, trace, attrs("span"), resource("resource"), scope("scope"),
		trace, resource("other"), scope("other"), trace, attrs("span"), resource("resource"), scope("scope")))
	for _, table := range []string{"events", "links"} {
		owner := strings.TrimSuffix(table, "s")
		timestamp := ""
		if table == "events" {
			timestamp = ", timestamp:=110::ubigint"
		}
		execSQL(fmt.Sprintf(`update spans set %s=case when span_id=1 then
			[struct_pack(id:=uuid(),attributes:=%s%s),struct_pack(id:=uuid(),attributes:=%s%s)]
			else [struct_pack(id:=uuid(),attributes:=case when span_id=2 then %s else %s end%s)] end`,
			table, attrs(owner), timestamp, attrs(owner), timestamp, attrs("other"), attrs(owner), timestamp))
	}
	execSQL(fmt.Sprintf(`insert into logs(id, timestamp, observed_timestamp, attributes, resource, scope)
		values ('%s', 0, 100, %s, %s, %s)`, logID, attrs("log"), resource("resource"), scope("scope")))
	execSQL(fmt.Sprintf(`insert into metric_records(record_id,id,resource_id,resource_payload_id,scope_id,name,metric_type,metadata,resource,scope,datapoints,point_time_min,point_time_max)
		values (uuid(),'%s',uuid(),uuid(),uuid(),'test','Gauge',%s,%s,%s,
		[struct_pack(id:='%s'::uuid,timestamp:=100::ubigint,attribute_key:='test',attributes:=%s,
		exemplars:=[struct_pack(id:=uuid(),timestamp:=110::ubigint,attributes:=%s),
		struct_pack(id:=uuid(),timestamp:=110::ubigint,attributes:=%s)])],100,100)`,
		metric, attrs("metadata"), resource("resource"), scope("scope"), point, attrs("datapoint"), attrs("exemplar"), attrs("exemplar")))

	type matchCase struct {
		name    string
		options map[string]any
		want    string
	}
	var cases []matchCase
	for _, location := range []struct {
		signal string
		owners []string
		ref    string
	}{
		{"traces", []string{"span", "event", "link", "resource", "scope"}, strings.ReplaceAll(trace, "-", "")},
		{"logs", []string{"log", "resource", "scope"}, logID},
		{"metrics", []string{"datapoint", "exemplar", "metadata", "resource", "scope"}, metric},
	} {
		for _, owner := range location.owners {
			for _, value := range []struct {
				key, kind, identity string
				missing             bool
			}{
				{"owner", "string", `{"kind":"string","value":"` + owner + `"}`, false},
				{"owner", "string", `{"kind":"string","value":"other-owner"}`, true},
				{"literal.dot", "string", `{"kind":"string","value":"literal"}`, false},
				{"literal.dot", "string", `{"kind":"string","value":"nested"}`, true},
				{"quote'key", "string", `{"kind":"string","value":"quote"}`, false},
				{"", "string", `{"kind":"string","value":"empty key"}`, false},
				{"maximum", "int64", `{"kind":"int64","value":"9223372036854775807"}`, false},
				{"maximum", "int64", `{"kind":"int64","value":"9223372036854775806"}`, true},
				{"maximum", "string", "", true},
				{"bytes", "bytes", `{"kind":"bytes","value":"AAH/"}`, false},
				{"negative_zero", "double", `{"kind":"double","value":"0x8000000000000000"}`, false},
				{"positive_zero", "double", `{"kind":"double","value":"0x8000000000000000"}`, true},
				{"empty", "empty", `{"kind":"empty","value":null}`, false},
				{"missing", "empty", `{"kind":"empty","value":null}`, true},
				{"empty_string", "string", `{"kind":"string","value":""}`, false},
				{"empty_array", "array", `{"kind":"array","value":[]}`, false},
				{"empty_map", "map", `{"kind":"map","value":[]}`, false},
				{"empty_bytes", "bytes", `{"kind":"bytes","value":""}`, false},
				{"array", "array", `{"kind":"array","value":[{"kind":"int64","value":"9223372036854775807"},{"kind":"double","value":"0x8000000000000000"},{"kind":"empty","value":null}]}`, false},
				{"map", "map", `{"kind":"map","value":[{"key":"child","value":{"kind":"string","value":"value"}}]}`, false},
				{"escaped", "string", "{\"kind\":\"string\",\"value\":\"quote' <>&\u2028\u2029\"}", false},
				{"empty", "empty", "", false},
				{"missing", "empty", "", true},
			} {
				options := map[string]any{"signal": location.signal, "ownerType": owner, "key": value.key, "kind": value.kind, "startTime": "90", "endTime": "120"}
				if value.identity != "" {
					options["valueIdentity"] = value.identity
				}
				want := location.ref
				if value.missing {
					want = ""
				}
				cases = append(cases, matchCase{location.signal + "/" + owner + "/" + value.key + "/" + value.identity, options, want})
			}
			if owner == "event" || owner == "exemplar" {
				for _, bounds := range [][2]string{{"90", "105"}, {"105", "120"}} {
					cases = append(cases, matchCase{location.signal + "/" + owner + "/time", map[string]any{
						"signal": location.signal, "ownerType": owner, "key": "owner", "kind": "string", "startTime": bounds[0], "endTime": bounds[1],
					}, ""})
				}
			}
		}
	}
	options := make([]map[string]any, len(cases))
	for i, tc := range cases {
		options[i] = tc.options
	}
	input, err := json.Marshal(options)
	require.NoError(t, err)
	cmd := exec.Command(node, "--input-type=module", "-e", `
		import { readFileSync } from 'node:fs';
		import { attributeMatchesSQL } from '../../frontend/src/services/attribute-matches.ts';
		console.log(JSON.stringify(JSON.parse(readFileSync(0, 'utf8')).map(options => attributeMatchesSQL(options, 0))));`)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	var statements []string
	require.NoError(t, json.Unmarshal(output, &statements))
	require.Len(t, statements, len(cases))
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.Query(statements[i])
			require.NoError(t, err, statements[i])
			defer rows.Close()
			if tc.want == "" {
				require.False(t, rows.Next())
			} else {
				require.True(t, rows.Next())
				var ref string
				var spans any
				var count int
				require.NoError(t, rows.Scan(&ref, &spans, &count))
				require.Equal(t, tc.want, ref)
				// The second span has identical values but a different owner marker.
				wantCount := 1
				if tc.options["signal"] == "traces" && tc.options["ownerType"] != "span" && tc.options["key"] != "owner" {
					wantCount = 2
				}
				require.Equal(t, wantCount, count)
				if tc.options["signal"] == "traces" {
					wantSpans := []any{"0000000000000001"}
					if wantCount == 2 {
						wantSpans = append(wantSpans, "0000000000000002")
					}
					require.Equal(t, wantSpans, spans)
				}
				require.False(t, rows.Next())
			}
			require.NoError(t, rows.Err())
		})
	}
}
