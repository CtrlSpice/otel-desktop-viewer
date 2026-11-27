package cli

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const traceFixture = `{"trace":{"traceID":"0123456789abcdef0123456789abcdef","spanCount":2,"logCount":2,"startTime":"18446744073709551000","durationNs":"500"},"spans":[{"spanID":"0000000000000001","parentSpanID":null,"service":"checkout","name":"root","startOffsetNs":"0","durationNs":"500"},{"spanID":"0000000000000002","parentSpanID":"ffffffffffffffff","service":"worker","name":"dangling","startOffsetNs":"615","durationNs":"-5"}],"logs":[{"timestamp":"18446744073709551615","spanID":null,"severity":"INFO","service":"checkout","eventName":"","body":"{order: 42}"},{"timestamp":"18446744073709551614","spanID":"ffffffffffffffff","severity":"WARN","service":"worker","eventName":"retry","body":"detached link"}]}`

var traceExpectedParams = map[string]any{"traceID": "0123456789abcdef0123456789abcdef"}

func TestTraceCommandJSONPreservesExactResult(t *testing.T) {
	server := rpcResultServer(t, "getTraceOverview", traceExpectedParams, json.RawMessage(traceFixture))
	out, _, err := execCLI(t, cliOptions{client: server.Client()},
		"trace", "01234567-89AB-CDEF-0123-456789ABCDEF", "--endpoint", server.URL, "--json")
	require.NoError(t, err)
	require.Equal(t, traceFixture+"\n", out)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.ElementsMatch(t, []string{"trace", "spans", "logs"}, mapKeys(result))
	require.ElementsMatch(t, []string{"traceID", "spanCount", "logCount", "startTime", "durationNs"}, mapKeys(result["trace"].(map[string]any)))
	require.ElementsMatch(t, []string{"spanID", "parentSpanID", "service", "name", "startOffsetNs", "durationNs"}, mapKeys(result["spans"].([]any)[0].(map[string]any)))
	require.ElementsMatch(t, []string{"timestamp", "spanID", "severity", "service", "eventName", "body"}, mapKeys(result["logs"].([]any)[0].(map[string]any)))
}

func TestTraceCommandCompactOutputDocumentsDerivedValues(t *testing.T) {
	server := rpcResultServer(t, "getTraceOverview", traceExpectedParams, json.RawMessage(traceFixture))
	out, _, err := execCLI(t, cliOptions{client: server.Client()},
		"trace", "0123456789abcdef0123456789abcdef", "--endpoint", server.URL)
	require.NoError(t, err)
	for _, want := range []string{
		"TRACE\n", "SPANS\n", "TRACE LOGS\n",
		"18446744073709551000", "18446744073709551615",
		"spanID            parentSpanID      service",
		"startOffsetNs", "durationNs", "NULL",
	} {
		require.Contains(t, out, want)
	}
	require.NotContains(t, out, "severityNumber")
	require.NotContains(t, out, "attributes")
}

func TestTraceCommandRejectsMalformedIDBeforeTransport(t *testing.T) {
	_, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, "trace", "not-a-trace")
	require.ErrorContains(t, err, "invalid trace ID")
}

func TestTraceCommandReturnsEveryCompactSpan(t *testing.T) {
	spans := make([]traceSpan, 6000)
	for i := range spans {
		spans[i] = traceSpan{SpanID: fmt.Sprintf("%016x", i+1), Name: fmt.Sprintf("span-%d", i), StartOffsetNs: fmt.Sprint(i), DurationNs: "1"}
	}
	raw, err := json.Marshal(traceResult{Trace: traceSummary{TraceID: "0123456789abcdef0123456789abcdef", SpanCount: int64(len(spans)), StartTime: "1", DurationNs: "6000"}, Spans: spans, Logs: []traceLog{}})
	require.NoError(t, err)

	server := rpcResultServer(t, "getTraceOverview", traceExpectedParams, raw)
	out, _, err := execCLI(t, cliOptions{client: server.Client()},
		"trace", "0123456789abcdef0123456789abcdef", "--endpoint", server.URL)
	require.NoError(t, err)
	require.Contains(t, out, "span-5999")
	require.NotContains(t, out, "more rows available")
}

func TestFormatTraceKeepsExactDerivedIntegers(t *testing.T) {
	result, err := decodeTrace(json.RawMessage(traceFixture))
	require.NoError(t, err)
	output := formatTrace(result)
	require.Contains(t, output, "-5")
	require.Contains(t, output, "18446744073709551615")
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
