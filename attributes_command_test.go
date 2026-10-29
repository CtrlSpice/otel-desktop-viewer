package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

// Execute the CLI's actual SQL with the pinned DuckDB engine. The HTTP fixture
// supplies the existing query response envelope and its one-row look-ahead.
func attributeTestViewer(t *testing.T) (*sql.DB, *httptest.Server) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE spans (trace_id UUID, span_id UBIGINT, start_time UBIGINT,
    service_name VARCHAR, attributes VARIANT, resource STRUCT(attributes VARIANT), PRIMARY KEY (trace_id, span_id));
-- This CLI-only fixture embeds tagged values; production integration tests
-- exercise the native-value encoder registered by the store.
CREATE MACRO otel_value_json(v) AS case when variant_exists(v, 'kind') then v::json else
    json_object('kind', 'map', 'value', coalesce((select list(json_object('key', key, 'value', value) order by key)
        from json_each(v::json)), []::json[])) end;
CREATE MACRO otel_value_kind(v) AS case when variant_typeof(v) = 'VARCHAR' then 'string' else v.kind::varchar end;
-- Mirror the native field accessor's contract for this tagged-value fixture.
CREATE MACRO attribute_native_field(parts, key_) AS json_extract(
    variant_bytes_to_variant(parts.metadata || parts.value)::JSON,
    '/' || replace(replace(key_, '~', '~0'), '/', '~1'))::VARIANT`)
	require.NoError(t, err)
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params struct {
				SQL   string `json:"sql"`
				Limit int    `json:"limit"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		assert.Equal(t, "query", request.Method)
		rows, err := db.QueryContext(r.Context(), "SELECT entry::VARCHAR FROM ("+request.Params.SQL+fmt.Sprintf(") LIMIT %d", request.Params.Limit+1))
		if err != nil {
			t.Error(err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		entries := make([][]json.RawMessage, 0)
		for rows.Next() {
			var entry string
			if err := rows.Scan(&entry); err != nil {
				t.Error(err)
				return
			}
			entries = append(entries, []json.RawMessage{json.RawMessage(entry)})
		}
		require.NoError(t, rows.Err())
		truncated := len(entries) > request.Params.Limit
		if truncated {
			entries = entries[:request.Params.Limit]
		}
		err = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{
				"columns": []queryColumn{{Name: "entry", Type: "JSON"}},
				"rows":    entries, "truncated": truncated,
			},
		})
		assert.NoError(t, err)
	}))
	t.Cleanup(viewer.Close)
	return db, viewer
}

func attributeTestRun(t *testing.T, endpoint string, args ...string) string {
	t.Helper()
	cmd := newAttributesCommand(http.DefaultClient, func() time.Time {
		return time.Unix(10000, 0)
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(append(args, "--endpoint", endpoint))
	require.NoError(t, cmd.Execute())
	return output.String()
}

func TestAttributeCommandsCountOwnersAndPreserveKinds(t *testing.T) {
	db, viewer := attributeTestViewer(t)
	_, err := db.Exec(`INSERT INTO spans (trace_id, span_id, start_time, service_name, attributes) VALUES
 ('00000000-0000-0000-0000-000000000001', 1, 6400000000000, 'checkout', '{"method":{"kind":"string","value":"GET"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 1, 10000000000000, 'checkout', '{"method":{"kind":"string","value":"GET"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 2, 7000000000000, 'checkout', '{"method":{"kind":"int64","value":"9007199254740993"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 3, 7000000000000, 'checkout', '{}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 4, 6399999999999, 'checkout', '{"method":{"kind":"string","value":"POST"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 5, 10000000000001, 'checkout', '{"method":{"kind":"string","value":"POST"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 6, 7000000000000, 'other', '{"method":{"kind":"string","value":"POST"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 7, 7000000000000, 'checkout', '{"method":{"kind":"string","value":"POST"}}'::json::variant),
 ('00000000-0000-0000-0000-000000000002', 8, 7000000000000, 'checkout', '{"method":{"kind":"string","value":"9007199254740993"}}'::json::variant)`)
	require.NoError(t, err)
	var keys attributeKeysResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, viewer.URL, "keys", "--json", "--service", "checkout")), &keys))
	assert.Equal(t, []attributeKey{
		{Key: "method", Kind: "int64", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
		{Key: "method", Kind: "string", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
	}, keys.Keys)
	assert.False(t, keys.Truncated)

	var values attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, viewer.URL, "values", "method", "--service", "checkout", "--json")), &values))
	require.Len(t, values.Values, 4)
	assert.False(t, values.Truncated)
	assert.JSONEq(t, `{"kind":"string","value":"GET"}`, string(values.Values[0].Value))
	assert.Equal(t, uint64(2), values.Values[0].Count, "same span ID in different traces counts twice")
	assert.Equal(t, 2.0/5.0, values.Values[0].RelativeFrequency)
	for _, value := range values.Values {
		assert.Equal(t, uint64(5), value.Denominator, "exclude spans without the key and apply the same service and time scope")
		assert.Equal(t, []attributeLocation{{Signal: "traces", OwnerType: "span"}}, value.FoundOn)
	}
	assert.JSONEq(t, `{"kind":"int64","value":"9007199254740993"}`, string(values.Values[1].Value))
	assert.JSONEq(t, `{"kind":"string","value":"9007199254740993"}`, string(values.Values[2].Value))

	for _, test := range []struct {
		limit     string
		count     int
		truncated bool
	}{{"1", 1, true}, {"4", 4, false}, {"5", 4, false}} {
		var limited attributeValuesResult
		require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, viewer.URL, "values", "method", "--service", "checkout", "--json", "--limit", test.limit)), &limited))
		require.Len(t, limited.Values, test.count)
		assert.Equal(t, test.truncated, limited.Truncated)
		assert.Equal(t, uint64(5), limited.Values[0].Denominator, "limit must not change the denominator")
	}
	output := attributeTestRun(t, viewer.URL, "values", "method", "--service", "checkout", "--limit", "1")
	for _, text := range []string{"value", "kind", "count", "percentage", `"GET"`, "40.00%", "more rows available"} {
		assert.Contains(t, output, text)
	}
	assert.NotContains(t, output, "█")
	assert.NotContains(t, output, "▇")
	assert.NotContains(t, output, "POST")
	assert.JSONEq(t, `{"key":"missing","values":[],"truncated":false}`, attributeTestRun(t, viewer.URL, "values", "missing", "--json"))
}

func TestAttributeCommandsPreserveNestedAndSpecialValuesAndQuoteFilters(t *testing.T) {
	db, viewer := attributeTestViewer(t)
	key := "quote'\\; --\nkey"
	service := "checkout' OR true --"
	values := []string{
		`{"kind":"empty","value":null}`,
		`{"kind":"int64","value":"9223372036854775807"}`,
		`{"kind":"double","value":"0x8000000000000000"}`,
		`{"kind":"double","value":"0x7ff8000000000001"}`,
		`{"kind":"bytes","value":"AAEC"}`,
		`{"kind":"map","value":[{"key":"dup","value":{"kind":"string","value":"a"}},{"key":"dup","value":{"kind":"int64","value":"9007199254740993"}}]}`,
		`{"kind":"array","value":[{"kind":"bool","value":true},{"kind":"string","value":"\\u001b"}]}`,
	}
	for i, value := range values {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		_, err := db.Exec("INSERT INTO spans (trace_id, span_id, start_time, service_name, attributes) VALUES (?::UUID, ?, 8000000000000, ?, json_object(?, ?::JSON)::variant)", id, i+1, service, key, value)
		require.NoError(t, err)
	}
	var result attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, viewer.URL, "values", key, "--service", service, "--json")), &result))
	require.Len(t, result.Values, len(values))
	actual := make([]any, 0, len(result.Values))
	for _, value := range result.Values {
		var decoded any
		require.NoError(t, json.Unmarshal(value.Value, &decoded))
		actual = append(actual, decoded)
		assert.Equal(t, uint64(len(values)), value.Denominator)
	}
	expected := make([]any, 0, len(values))
	for _, value := range values {
		var decoded any
		require.NoError(t, json.Unmarshal([]byte(value), &decoded))
		expected = append(expected, decoded)
	}
	assert.ElementsMatch(t, expected, actual)
	assert.JSONEq(t, `{"keys":[],"truncated":false}`, attributeTestRun(t, viewer.URL, "keys", "--service", "absent", "--json"))
}

func TestAttributesOfflineHelpAndValidation(t *testing.T) {
	for _, args := range [][]string{{"attributes"}, {"attributes", "--help"}, {"attributes", "keys", "--help"}, {"attributes", "values", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			client := &http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("help made an HTTP request")
				return nil, nil
			})}
			cmd := newRootCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}}, client, time.Now,
				func(context.Context, otelcol.CollectorSettings) error { t.Fatal("help started a viewer"); return nil },
				func(string) error { t.Fatal("help opened a browser"); return nil })
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(args)
			require.NoError(t, cmd.Execute())
			assert.Contains(t, output.String(), "USAGE")
			if len(args) < 3 {
				assert.Contains(t, output.String(), "keys")
				assert.Contains(t, output.String(), "values")
			} else {
				for _, flag := range []string{"--limit", "--signal", "--owner-type", "--start", "--end", "--since", "--service", "--json", "--endpoint"} {
					assert.Contains(t, output.String(), flag)
				}
			}
		})
	}
	for _, args := range [][]string{{"values"}, {"keys", "extra"}, {"keys", "--signal", "logs"}, {"values", "key", "--owner-type", "datapoint"}, {"keys", "--signal", "unknown"}, {"keys", "--limit", "0"}, {"keys", "--since", "1h", "--start", "2026-10-02T00:00:00Z"}} {
		cmd := newAttributesCommand(&http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid input made an HTTP request")
			return nil, nil
		})}, time.Now)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		require.Error(t, cmd.Execute())
	}
}

func TestAttributeSkillRecordLookup(t *testing.T) {
	db, _ := attributeTestViewer(t)
	for _, name := range []string{"trace_id_wire", "span_id_wire"} {
		macro, err := os.ReadFile("desktopexporter/internal/store/queries/ddl/macros/" + name + ".sql")
		require.NoError(t, err)
		_, err = db.Exec(string(macro))
		require.NoError(t, err)
	}
	_, err := db.Exec(`ALTER TABLE spans ADD COLUMN name VARCHAR DEFAULT 'request';
INSERT INTO spans (trace_id, span_id, start_time, service_name, attributes) VALUES
 ('00000000-0000-0000-0000-000000000001', 42, 1790928000000000000, 'checkout', {'http.request.method':'POST'}::variant),
 ('00000000-0000-0000-0000-000000000002', 42, 1790931600000000000, 'checkout', {'http.request.method':'POST'}::variant),
 ('00000000-0000-0000-0000-000000000003', 42, 1790931600000000000, 'checkout', {'http.request.method':'GET'}::variant),
 ('00000000-0000-0000-0000-000000000004', 42, 1790931600000000001, 'checkout', {'http.request.method':'POST'}::variant),
 ('00000000-0000-0000-0000-000000000005', 42, 1790931600000000000, 'other', {'http.request.method':'POST'}::variant),
 ('00000000-0000-0000-0000-000000000006', 42, 1790931600000000000, 'checkout', {'other.key':'POST'}::variant)`)
	require.NoError(t, err)
	guide, err := os.ReadFile("skills/otel-desktop-viewer/SKILL.md")
	require.NoError(t, err)
	_, section, ok := strings.Cut(strings.ReplaceAll(string(guide), "\r\n", "\n"), "## Discover attributes and inspect matching records")
	require.True(t, ok)
	_, statement, ok := strings.Cut(section, "otel-desktop-viewer query --json \"\n")
	require.True(t, ok)
	statement, _, ok = strings.Cut(statement, "\"\n```")
	require.True(t, ok)
	rows, err := db.Query(statement)
	require.NoError(t, err)
	defer rows.Close()
	var traces []string
	for rows.Next() {
		var traceID, spanID, service, name string
		require.NoError(t, rows.Scan(&traceID, &spanID, &service, &name))
		assert.Equal(t, "000000000000002a", spanID)
		assert.Equal(t, "checkout", service)
		traces = append(traces, traceID)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"00000000000000000000000000000002", "00000000000000000000000000000001"}, traces)
}
