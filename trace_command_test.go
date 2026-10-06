package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const traceFixture = `{"trace":{"traceID":"0123456789abcdef0123456789abcdef","spanCount":2,"logCount":2,"startTime":"18446744073709551000","durationNs":"500"},"spans":[{"spanID":"0000000000000001","parentSpanID":null,"service":"checkout","name":"root","startOffsetNs":"0","durationNs":"500"},{"spanID":"0000000000000002","parentSpanID":"ffffffffffffffff","service":"worker","name":"dangling","startOffsetNs":"615","durationNs":"-5"}],"logs":[{"timestamp":"18446744073709551615","spanID":null,"severity":"INFO","service":"checkout","eventName":"","body":"{order: 42}"},{"timestamp":"18446744073709551614","spanID":"ffffffffffffffff","severity":"WARN","service":"worker","eventName":"retry","body":"detached link"}]}`

func TestTraceCommandJSONPreservesExactResult(t *testing.T) {
	server := traceRPCServer(t, json.RawMessage(traceFixture))
	defer server.Close()
	cmd := newTraceCommand(server.Client())
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"01234567-89AB-CDEF-0123-456789ABCDEF", "--endpoint", server.URL, "--json"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, traceFixture+"\n", output.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.ElementsMatch(t, []string{"trace", "spans", "logs"}, mapKeys(result))
	require.ElementsMatch(t, []string{"traceID", "spanCount", "logCount", "startTime", "durationNs"}, mapKeys(result["trace"].(map[string]any)))
	require.ElementsMatch(t, []string{"spanID", "parentSpanID", "service", "name", "startOffsetNs", "durationNs"}, mapKeys(result["spans"].([]any)[0].(map[string]any)))
	require.ElementsMatch(t, []string{"timestamp", "spanID", "severity", "service", "eventName", "body"}, mapKeys(result["logs"].([]any)[0].(map[string]any)))
}

func TestTraceCommandCompactOutputDocumentsDerivedValues(t *testing.T) {
	server := traceRPCServer(t, json.RawMessage(traceFixture))
	defer server.Close()
	cmd := newTraceCommand(server.Client())
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"0123456789abcdef0123456789abcdef", "--endpoint", server.URL})
	require.NoError(t, cmd.Execute())
	require.Contains(t, output.String(), "TRACE\n")
	require.Contains(t, output.String(), "SPANS\n")
	require.Contains(t, output.String(), "TRACE LOGS\n")
	require.Contains(t, output.String(), "18446744073709551000")
	require.Contains(t, output.String(), "18446744073709551615")
	require.Contains(t, output.String(), "spanID            parentSpanID      service")
	require.Contains(t, output.String(), "startOffsetNs")
	require.Contains(t, output.String(), "durationNs")
	require.NotContains(t, output.String(), "severityNumber")
	require.NotContains(t, output.String(), "attributes")
	require.Contains(t, output.String(), "NULL")
}

func TestTraceCommandRejectsMalformedIDBeforeTransport(t *testing.T) {
	cmd := newTraceCommand(http.DefaultClient)
	cmd.SetArgs([]string{"not-a-trace"})
	require.ErrorContains(t, cmd.Execute(), "invalid trace ID")
}

func TestTraceCommandReturnsEveryCompactSpan(t *testing.T) {
	spans := make([]traceSpan, 6000)
	for i := range spans {
		spans[i] = traceSpan{SpanID: fmt.Sprintf("%016x", i+1), Name: fmt.Sprintf("span-%d", i), StartOffsetNs: fmt.Sprint(i), DurationNs: "1"}
	}
	raw, err := json.Marshal(traceResult{Trace: traceSummary{TraceID: "0123456789abcdef0123456789abcdef", SpanCount: int64(len(spans)), StartTime: "1", DurationNs: "6000"}, Spans: spans, Logs: []traceLog{}})
	require.NoError(t, err)
	server := traceRPCServer(t, raw)
	defer server.Close()
	cmd := newTraceCommand(server.Client())
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"0123456789abcdef0123456789abcdef", "--endpoint", server.URL})
	require.NoError(t, cmd.Execute())
	require.Contains(t, output.String(), "span-5999")
	require.NotContains(t, output.String(), "more rows available")
}

func TestFormatTraceKeepsExactDerivedIntegers(t *testing.T) {
	result, err := decodeTrace(json.RawMessage(traceFixture))
	require.NoError(t, err)
	output := formatTrace(result)
	require.Contains(t, output, "-5")
	require.Contains(t, output, "18446744073709551615")
}

func traceRPCServer(t *testing.T, result json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpc queryRPCRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&rpc))
		require.Equal(t, "getTraceOverview", rpc.Method)
		require.Equal(t, "0123456789abcdef0123456789abcdef", rpc.Params["traceID"])
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(append(append([]byte(`{"jsonrpc":"2.0","id":1,"result":`), result...), '}'))
	}))
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
