package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const foundSpanFixture = `{"status":"found","traceID":"0123456789abcdef0123456789abcdef","span":{"traceID":"0123456789abcdef0123456789abcdef","traceState":"vendor=value","spanID":"000000000000002a","parentSpanID":null,"flags":4294967295,"name":"POST /checkout","kindCode":2,"kind":"Server","startTime":"18446744073709551000","endTime":"18446744073709551615","attributes":[{"key":"attempt","value":{"kind":"int64","value":"9223372036854775807"}}],"events":[{"name":"retry","timestamp":"18446744073709551614","droppedAttributesCount":1,"attributes":[{"key":"ratio","value":{"kind":"double","value":{"bits":"8000000000000000"}}}]}],"links":[{"traceID":null,"spanID":null,"traceState":"","droppedAttributesCount":2,"flags":3,"attributes":[]}],"resource":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":4},"scope":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":5},"resourceSchemaURL":"resource-schema","scopeSchemaURL":"scope-schema","droppedAttributesCount":6,"droppedEventsCount":7,"droppedLinksCount":8,"statusCodeValue":2,"statusCode":"Error","statusMessage":"failed"},"logs":[{"id":"00000000-0000-0000-0000-000000000001","timestamp":"0","observedTimestamp":"18446744073709551615","traceID":"0123456789abcdef0123456789abcdef","spanID":"000000000000002a","severityText":"INFO","severityNumber":9,"body":{"kind":"map","value":[{"key":"duplicate","value":{"kind":"double","value":{"bits":"8000000000000000"}}},{"key":"duplicate","value":{"kind":"double","value":{"bits":"7ff0000000000000"}}}]},"resource":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":10},"scope":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":11},"resourceSchemaURL":"log-resource-schema","scopeSchemaURL":"log-scope-schema","droppedAttributesCount":12,"flags":13,"eventName":"event","attributes":[]}]}`

func TestSpanCommandOneAndTwoArgumentsPreserveJSON(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		params map[string]any
	}{
		{[]string{"00000000000000AA"}, map[string]any{"spanID": "00000000000000aa"}},
		{[]string{"01234567-89AB-CDEF-0123-456789ABCDEF", "00000000000000AA"}, map[string]any{"traceID": "0123456789abcdef0123456789abcdef", "spanID": "00000000000000aa"}},
	} {
		server := spanRPCServer(t, json.RawMessage(foundSpanFixture), tc.params)
		defer server.Close()
		cmd := newSpanCommand(server.Client())
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append(tc.args, "--endpoint", server.URL, "--json"))
		require.NoError(t, cmd.Execute())
		require.Equal(t, foundSpanFixture+"\n", out.String())
	}
}

func TestSpanCommandValidationAndOutcomes(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b", "c"}, {"0"}, {"000000000000000g"}} {
		cmd := newSpanCommand(http.DefaultClient)
		cmd.SetArgs(args)
		require.Error(t, cmd.Execute())
	}
	require.Equal(t, "0000000000000000", mustNormalizeSpanID(t, "0000000000000000"))
	for _, tc := range []struct {
		result string
		wants  []string
	}{
		{`{"status":"notFound","spanID":"000000000000002a","traceID":null}`, []string{"was not found"}},
		{`{"status":"ambiguous","spanID":"000000000000002a","matchCount":2,"traceIDs":["00000000000000000000000000000001","00000000000000000000000000000002"]}`, []string{"occurs in 2 traces", "span 00000000000000000000000000000001 000000000000002a"}},
		{foundSpanFixture, []string{"SPAN\n", "TIMING AND STATUS\n", "RESOURCE ATTRIBUTES\n", "SPAN ATTRIBUTES\n", "EVENTS (1)\n", "LINKS (1)\n", "CORRELATED LOGS (1)\n", "int64", "9223372036854775807", "8000000000000000", "log-resource-schema"}},
	} {
		server := spanRPCServer(t, json.RawMessage(tc.result), map[string]any{"spanID": "000000000000002a"})
		cmd := newSpanCommand(server.Client())
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"000000000000002a", "--endpoint", server.URL})
		require.NoError(t, cmd.Execute())
		server.Close()
		for _, want := range tc.wants {
			require.Contains(t, out.String(), want)
		}
	}
}

func TestSpanCommandRejectsMalformedResultAndTrailingJSON(t *testing.T) {
	server := spanRPCServer(t, json.RawMessage(`{"status":"found"}`), map[string]any{"spanID": "000000000000002a"})
	cmd := newSpanCommand(server.Client())
	cmd.SetArgs([]string{"000000000000002a", "--endpoint", server.URL})
	require.ErrorContains(t, cmd.Execute(), "incomplete found result")
	server.Close()
	trailing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"notFound","spanID":"000000000000002a","traceID":null}} {}`))
	}))
	defer trailing.Close()
	cmd = newSpanCommand(trailing.Client())
	cmd.SetArgs([]string{"000000000000002a", "--endpoint", trailing.URL})
	require.ErrorContains(t, cmd.Execute(), "additional JSON value")
}

func mustNormalizeSpanID(t *testing.T, s string) string {
	t.Helper()
	got, err := normalizeSpanID(s)
	require.NoError(t, err)
	return got
}
func spanRPCServer(t *testing.T, result json.RawMessage, params map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req queryRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "getSpan", req.Method)
		require.Equal(t, params, req.Params)
		_, _ = w.Write(append(append([]byte(`{"jsonrpc":"2.0","id":1,"result":`), result...), '}'))
	}))
}
