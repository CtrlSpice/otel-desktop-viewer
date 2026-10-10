package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/otelcol"
)

func writeImportTestFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func importTestViewer(t *testing.T, receiver *httptest.Server) *httptest.Server {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(receiver.URL, "http://"))
	require.NoError(t, err)
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/rpc", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"method":"getImportConfig"`)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"otlpHttpPort":%s}}`, port)
	}))
	t.Cleanup(viewer.Close)
	return viewer
}

func TestImportCommandPortDiscoveryTablesAndFilePreservation(t *testing.T) {
	input := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"precise","startTimeUnixNano":18446744073709551615}]}]}]}`
	path := writeImportTestFile(t, input)
	var received string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/traces", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received = string(body)
		fmt.Fprint(w, `{}`)
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	root := newRootCommand(otelcol.CollectorSettings{}, viewer.Client(), nil, nil, nil)
	root.SetArgs([]string{"import", path, "--endpoint", viewer.URL})
	var output bytes.Buffer
	root.SetOut(&output)
	require.NoError(t, root.ExecuteContext(t.Context()))
	require.Equal(t, input, received)
	require.Contains(t, output.String(), "ACCEPTED REQUESTS")
	require.Contains(t, output.String(), "ingestion not confirmed")
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, input, string(unchanged))
	for _, args := range [][]string{{"--help"}, {"import", "--help"}} {
		root := newRootCommand(otelcol.CollectorSettings{}, viewer.Client(), nil, nil, nil)
		root.SetArgs(args)
		var help bytes.Buffer
		root.SetOut(&help)
		require.NoError(t, root.Execute())
		require.Contains(t, help.String(), "import")
		require.Contains(t, help.String(), "USAGE")
		if len(args) > 1 {
			require.Contains(t, help.String(), "FLAGS")
			require.Contains(t, help.String(), "--endpoint")
			require.Contains(t, help.String(), "EXAMPLES")
		}
	}
}

func TestImportCommandLateInvalidSyntaxAndLaterFilesContinue(t *testing.T) {
	bad := writeImportTestFile(t, `{"resourceLogs":[{}]}`+"\n"+`{"resourceSpans":[`)
	good := writeImportTestFile(t, `{"resourceLogs":[{}]}`)
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; fmt.Fprint(w, `{}`) }))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	cmd := newImportCommand(viewer.Client())
	cmd.SetArgs([]string{bad, good, "--endpoint", viewer.URL})
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.Error(t, cmd.ExecuteContext(t.Context()))
	require.Equal(t, 1, requests)
	require.Contains(t, output.String(), "invalid JSON")
	require.Contains(t, output.String(), "ingestion not confirmed")
}

func TestImportResponseIssues(t *testing.T) {
	for _, test := range []struct {
		body    string
		status  int
		issue   string
		invalid bool
	}{
		{`{}`, 200, "", false},
		{`{"partialSuccess":{}}`, 200, "", false},
		{`{"partialSuccess":{"rejectedSpans":"000"}}`, 200, "", false},
		{`{"partialSuccess":{"rejectedLogRecords":"18446744073709551615"}}`, 200, "18446744073709551615", false},
		{`{"partialSuccess":{"rejectedDataPoints":2,"errorMessage":"bad points"}}`, 200, "bad points", false},
		{`{"partialSuccess":{"errorMessage":"warning"}}`, 200, "warning", false},
		{`{"partialSuccess":{"rejectedSpans":-1}}`, 200, "", true},
		{`{"partialSuccess":{"rejectedSpans":9007199254740992}}`, 200, "", true},
		{`{"partialSuccess":{"rejectedSpans":"1.5"}}`, 200, "", true},
		{`{"partialSuccess":{"errorMessage":null}}`, 200, "", true},
		{`{"partialSuccess":null}`, 200, "", true},
		{`[]`, 200, "", true},
		{``, 200, "", true},
		{`{"message":" receiver error "}`, 400, "receiver error", false},
		{`proxy error`, 502, "proxy error", false},
	} {
		t.Run(test.body, func(t *testing.T) {
			response := &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}
			issue, err := importResponseIssue(response)
			if test.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Contains(t, issue, test.issue)
			}
		})
	}
}

func TestImportReceiverFailureContinuesSignalsAndCancellation(t *testing.T) {
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/v1/traces" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"message":"invalid trace"}`)
		} else {
			fmt.Fprint(w, `{"partialSuccess":{"rejectedLogRecords":"1"}}`)
		}
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	file := writeImportTestFile(t, `{"resourceSpans":[{}],"resourceLogs":[{}]}`)
	accepted, issues, err := importOTLPFile(t.Context(), viewer.Client(), viewer.URL, file)
	require.NoError(t, err)
	require.Zero(t, accepted)
	require.Equal(t, 2, requests)
	require.Len(t, issues, 2)
	require.Contains(t, issues[0], "invalid trace")
	require.Contains(t, issues[1], "rejected 1")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = importOTLPFile(ctx, viewer.Client(), viewer.URL, file)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, requests)
}

func TestImportConfigValidation(t *testing.T) {
	for _, result := range []string{`null`, `{}`, `{"otlpHttpPort":0}`, `{"otlpHttpPort":65536}`, `{"otlpHttpPort":"4318"}`, `{"otlpHttpPort":1.5}`} {
		viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, result)
		}))
		_, _, err := importOTLPFile(t.Context(), viewer.Client(), viewer.URL, writeImportTestFile(t, `{"resourceLogs":[{}]}`))
		require.ErrorContains(t, err, "invalid OTLP HTTP port")
		viewer.Close()
	}
}

func TestImportCancellationStopsInFlightRequestAndLaterSignals(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, err := io.Copy(io.Discard, r.Body)
		require.NoError(t, err)
		cancel()
		<-r.Context().Done()
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	path := writeImportTestFile(t, `{"resourceSpans":[{}],"resourceLogs":[{}]}`)
	accepted, _, err := importOTLPFile(ctx, viewer.Client(), viewer.URL, path)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, accepted)
	require.Equal(t, 1, requests)
}
