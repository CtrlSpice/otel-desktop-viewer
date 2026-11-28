package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const foundSpanFixture = `{"status":"found","traceID":"0123456789abcdef0123456789abcdef","span":{"traceID":"0123456789abcdef0123456789abcdef","traceState":"vendor=value","spanID":"000000000000002a","parentSpanID":null,"flags":4294967295,"name":"POST /checkout","kindCode":2,"kind":"Server","startTime":"18446744073709551000","endTime":"18446744073709551615","attributes":[{"key":"attempt","value":{"kind":"int64","value":"9223372036854775807"}}],"events":[{"name":"retry","timestamp":"18446744073709551614","droppedAttributesCount":1,"attributes":[{"key":"ratio","value":{"kind":"double","value":{"bits":"8000000000000000"}}}]}],"links":[{"traceID":null,"spanID":null,"traceState":"","droppedAttributesCount":2,"flags":3,"attributes":[]}],"resource":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":4},"scope":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":5},"resourceSchemaURL":"resource-schema","scopeSchemaURL":"scope-schema","droppedAttributesCount":6,"droppedEventsCount":7,"droppedLinksCount":8,"statusCodeValue":2,"statusCode":"Error","statusMessage":"failed"},"logs":[{"logRef":"00000000-0000-0000-0000-000000000001","timestamp":"0","observedTimestamp":"18446744073709551615","traceID":"0123456789abcdef0123456789abcdef","spanID":"000000000000002a","severityText":"INFO","severityNumber":9,"body":{"kind":"map","value":[{"key":"duplicate","value":{"kind":"double","value":{"bits":"8000000000000000"}}},{"key":"duplicate","value":{"kind":"double","value":{"bits":"7ff0000000000000"}}}]},"resource":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":10},"scope":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":11},"resourceSchemaURL":"log-resource-schema","scopeSchemaURL":"log-scope-schema","droppedAttributesCount":12,"flags":13,"eventName":"event","attributes":[]}]}`

const ambiguousSpanFixture = `{"status":"ambiguous","spanID":"000000000000002a","matchCount":2,"summaries":[{"traceID":"00000000000000000000000000000002","spanID":"000000000000002a","parentSpanID":null,"service":"worker","name":"second","startTime":"20","durationNs":"5"},{"traceID":"00000000000000000000000000000001","spanID":"000000000000002a","parentSpanID":"0000000000000001","service":"api","name":"first","startTime":"10","durationNs":"3"}],"truncated":false}`

func TestSpanCommandOneAndTwoArgumentsPreserveJSON(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		params map[string]any
	}{
		{[]string{"00000000000000AA"}, map[string]any{"spanID": "00000000000000aa", "limit": float64(25)}},
		{[]string{"01234567-89AB-CDEF-0123-456789ABCDEF", "00000000000000AA"}, map[string]any{"traceID": "0123456789abcdef0123456789abcdef", "spanID": "00000000000000aa"}},
	} {
		server := rpcResultServer(t, "getSpan", tc.params, json.RawMessage(foundSpanFixture))
		args := append([]string{"span"}, tc.args...)
		args = append(args, "--endpoint", server.URL, "--json")
		out, _, err := execCLI(t, cliOptions{client: server.Client()}, args...)
		require.NoError(t, err)
		require.Equal(t, foundSpanFixture+"\n", out)
	}
}

func TestSpanCommandValidationAndOutcomes(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b", "c"}, {"0"}, {"000000000000000g"}} {
		_, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, append([]string{"span"}, args...)...)
		require.Error(t, err)
	}
	got, err := normalizeSpanID("0000000000000000")
	require.NoError(t, err)
	require.Equal(t, "0000000000000000", got)

	for _, limit := range []string{"0", "-1", "9223372036854775807"} {
		_, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, "span", "000000000000002a", "--limit", limit)
		require.Error(t, err)
	}

	server := rpcResultServer(t, "getSpan", map[string]any{
		"spanID": "000000000000002a", "limit": float64(50),
	}, json.RawMessage(ambiguousSpanFixture))
	_, _, err = execCLI(t, cliOptions{client: server.Client()}, "span", "000000000000002a", "--limit", "50", "--endpoint", server.URL)
	require.NoError(t, err)

	foundWithScopeAttribute := strings.Replace(foundSpanFixture,
		`"scope":{"name":"scope","version":"1","attributes":[]`,
		`"scope":{"name":"scope","version":"1","attributes":[{"key":"scope.key","value":{"kind":"string","value":"scope value"}}]`, 1)
	for _, tc := range []struct {
		result string
		wants  []string
	}{
		{`{"status":"notFound","spanID":"000000000000002a","traceID":null}`, []string{"was not found"}},
		{ambiguousSpanFixture, []string{"SPAN SUMMARIES (2 matches)", "traceID", "spanID", "second", "durationNs"}},
		{foundWithScopeAttribute, []string{"SPAN\n", "TIMING AND STATUS\n", "RESOURCE ATTRIBUTES\n", "SCOPE ATTRIBUTES\n", "scope.key", "scope value", "SPAN ATTRIBUTES\n", "EVENTS (1)\n", "LINKS (1)\n", "CORRELATED LOGS (1)\n", "int64", "9223372036854775807", "8000000000000000", "log-resource-schema"}},
	} {
		server := rpcResultServer(t, "getSpan", map[string]any{"spanID": "000000000000002a", "limit": float64(25)}, json.RawMessage(tc.result))
		out, _, err := execCLI(t, cliOptions{client: server.Client()}, "span", "000000000000002a", "--endpoint", server.URL)
		require.NoError(t, err)
		for _, want := range tc.wants {
			require.Contains(t, out, want)
		}
	}
}

func TestSpanCommandRejectsMalformedResult(t *testing.T) {
	server := rpcResultServer(t, "getSpan", map[string]any{"spanID": "000000000000002a", "limit": float64(25)}, json.RawMessage(`{"status":"found"}`))
	_, _, err := execCLI(t, cliOptions{client: server.Client()}, "span", "000000000000002a", "--endpoint", server.URL)
	require.ErrorContains(t, err, "incomplete found result")
}
