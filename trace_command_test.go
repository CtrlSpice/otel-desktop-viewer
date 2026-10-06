package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const traceDetailFixture = `{"trace":{"traceID":"0123456789abcdef0123456789abcdef","traceStart":"18446744073709551000","resources":{"1":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":0}},"scopes":{"1":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":0}},"unplacedSpanCount":0,"spans":[{"spanData":{"traceState":"","spanID":"0000000000000001","parentSpanID":null,"flags":1,"name":"root","kindCode":2,"kind":"Server","start":"0","dur":"500","attributes":[{"key":"limit","value":{"kind":"int64","value":"9223372036854775807"}}],"events":[],"links":[],"r":1,"s":1,"resourceSchemaURL":"resource-schema","scopeSchemaURL":"scope-schema","droppedAttributesCount":0,"droppedEventsCount":0,"droppedLinksCount":0,"statusCodeValue":0,"statusCode":"Unset","statusMessage":""}}]},"logs":[{"id":"00000000-0000-0000-0000-000000000001","timestamp":"0","observedTimestamp":"18446744073709551615","traceID":"0123456789abcdef0123456789abcdef","spanID":null,"severityText":"INFO","severityNumber":9,"body":{"kind":"map","value":[{"key":"duplicate","value":{"kind":"double","value":{"bits":"8000000000000000"}}},{"key":"duplicate","value":{"kind":"double","value":{"bits":"7ff0000000000000"}}}]},"resource":{"attributes":[{"key":"service.name","value":{"kind":"string","value":"checkout"}}],"droppedAttributesCount":0},"scope":{"name":"scope","version":"1","attributes":[],"droppedAttributesCount":0},"resourceSchemaURL":"resource-schema","scopeSchemaURL":"scope-schema","droppedAttributesCount":4294967295,"flags":4294967295,"eventName":""}]}`

func TestTraceCommandJSONPreservesExactResult(t *testing.T) {
	server := traceRPCServer(t, json.RawMessage(traceDetailFixture))
	defer server.Close()
	cmd := newTraceCommand(server.Client())
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"01234567-89AB-CDEF-0123-456789ABCDEF", "--endpoint", server.URL, "--json"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, traceDetailFixture+"\n", output.String())
}

func TestTraceCommandCompactOutputDocumentsDerivedValues(t *testing.T) {
	server := traceRPCServer(t, json.RawMessage(traceDetailFixture))
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
	require.Contains(t, output.String(), "500")
	require.Contains(t, output.String(), "kindCode")
	require.Contains(t, output.String(), "NULL")
}

func TestTraceCommandReturnsEverySyntheticSpan(t *testing.T) {
	spans := make([]string, 6000)
	for i := range spans {
		spans[i] = fmt.Sprintf(`{"spanData":{"spanID":"%016x","parentSpanID":null,"name":"span-%d","kindCode":1,"kind":"Internal","start":"%d","dur":"1","r":1,"statusCode":"Unset"}}`, i+1, i, i)
	}
	raw := json.RawMessage(`{"trace":{"traceID":"0123456789abcdef0123456789abcdef","traceStart":"1","resources":{"1":{"attributes":[]}},"spans":[` + strings.Join(spans, ",") + `]},"logs":[]}`)
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

func TestTraceCommandRejectsMalformedIDBeforeTransport(t *testing.T) {
	cmd := newTraceCommand(http.DefaultClient)
	cmd.SetArgs([]string{"not-a-trace"})
	require.ErrorContains(t, cmd.Execute(), "invalid trace ID")
}

func traceRPCServer(t *testing.T, result json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpc queryRPCRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&rpc))
		require.Equal(t, "getTraceDetail", rpc.Method)
		require.Equal(t, "0123456789abcdef0123456789abcdef", rpc.Params["traceID"])
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(append(append([]byte(`{"jsonrpc":"2.0","id":1,"result":`), result...), '}'))
	}))
}
