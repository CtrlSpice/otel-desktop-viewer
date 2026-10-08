package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

func TestQueryCommandPrintsColumnsAndSendsSQL(t *testing.T) {
	var request queryRPCRequest
	viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
		assert.Equal(t, "/rpc", httpRequest.URL.Path)
		require.NoError(t, json.NewDecoder(httpRequest.Body).Decode(&request))
		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"columns":[{"name":"service","type":"VARCHAR"},{"name":"count","type":"BIGINT"}],"rows":[["checkout",9007199254740993],[null,0]],"truncated":true}}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"query", "select service_name, count(*) from spans group by service_name", "--endpoint", viewer.URL, "--limit", "2"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "service   count           \n--------  ----------------\ncheckout  9007199254740993\nNULL      0               \n[2 rows shown; more rows available; use --limit to return more]\n", output.String())
	assert.Equal(t, "query", request.Method)
	assert.Equal(t, "select service_name, count(*) from spans group by service_name", request.Params["sql"])
	assert.Equal(t, float64(2), request.Params["limit"])
}

func TestQueryCommandJSONPreservesIntegerTokensNullAndEmptyText(t *testing.T) {
	result := `{"columns":[{"name":"n","type":"BIGINT"},{"name":"text","type":"VARCHAR"}],"rows":[[9007199254740993,""],[0,null]],"truncated":false}`
	viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"query", "select n, text from values", "--endpoint", viewer.URL, "--json"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, result+"\n", output.String())
}

func TestQueryCommandDefaultLimitAndHelpAreOffline(t *testing.T) {
	requestReceived := make(chan queryRPCRequest, 1)
	viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
		var request queryRPCRequest
		require.NoError(t, json.NewDecoder(httpRequest.Body).Decode(&request))
		requestReceived <- request
		_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"columns":[],"rows":[],"truncated":false}}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	cmd.SetArgs([]string{"query", "select 1", "--endpoint", viewer.URL})
	require.NoError(t, cmd.Execute())
	request := <-requestReceived
	assert.Equal(t, float64(queryDefaultLimit), request.Params["limit"])

	cmd = newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"query", "--help", "--endpoint", "http://127.0.0.1:1"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, output.String(), "🦆 Run one read-only DuckDB query against the existing viewer process.")
	assert.Contains(t, output.String(), "SHOW TABLES")
	assert.Contains(t, output.String(), "--json")
	assert.Contains(t, output.String(), "--limit")

	cmd = newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	output.Reset()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, output.String(), "query     🦆 Run SQL against a running viewer")
}

func TestQueryCommandReturnsRPCAndHTTPFailuresWithoutUsage(t *testing.T) {
	t.Run("RPC error", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"query rejected: write statement"}}`))
			require.NoError(t, err)
		}))
		defer viewer.Close()

		cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs([]string{"query", "delete from spans", "--endpoint", viewer.URL})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "viewer query error -32602: query rejected: write statement")
		assert.NotContains(t, output.String(), "Usage:")
	})

	t.Run("HTTP error", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer viewer.Close()

		_, _, err := requestQuery(context.Background(), viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 503 Service Unavailable")
	})
}

func TestRequestQueryRequiresOneCompleteJSONRPCResponse(t *testing.T) {
	validResponse := `{"jsonrpc":"2.0","id":1,"result":{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[9007199254740993]],"truncated":false}}`

	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "malformed trailing data", response: validResponse + ` trailing`, wantErr: "decode viewer response: trailing data"},
		{name: "second JSON value", response: validResponse + ` {"jsonrpc":"2.0"}`, wantErr: "decode viewer response: additional JSON value"},
		{name: "trailing whitespace", response: validResponse + " \n\t\r"},
		{name: "ordinary response", response: validResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, err := writer.Write([]byte(test.response))
				require.NoError(t, err)
			}))
			defer viewer.Close()

			raw, result, err := requestQuery(context.Background(), viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.NoError(t, err)
			assert.JSONEq(t, `{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[9007199254740993]],"truncated":false}`, string(raw))
			require.Len(t, result.Rows, 1)
			assert.Equal(t, json.Number("9007199254740993"), result.Rows[0][0])
		})
	}
}

func TestRequestQueryRequiresMatchingJSONRPCIdentity(t *testing.T) {
	result := `"result":{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[9007199254740993]],"truncated":false}`
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "valid response", response: `{"jsonrpc":"2.0","id":1,` + result + `}`},
		{name: "missing version", response: `{"id":1,` + result + `}`, wantErr: "invalid jsonrpc version"},
		{name: "wrong version", response: `{"jsonrpc":"1.0","id":1,` + result + `}`, wantErr: "invalid jsonrpc version"},
		{name: "missing id", response: `{"jsonrpc":"2.0",` + result + `}`, wantErr: "response id does not match request id"},
		{name: "wrong numeric id", response: `{"jsonrpc":"2.0","id":2,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "null id", response: `{"jsonrpc":"2.0","id":null,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "string id", response: `{"jsonrpc":"2.0","id":"1",` + result + `}`, wantErr: "response id does not match request id"},
		{name: "non-integer numeric id", response: `{"jsonrpc":"2.0","id":1.0,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "valid error response", response: `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"query rejected"}}`, wantErr: "viewer query error -32602: query rejected"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, err := writer.Write([]byte(test.response))
				require.NoError(t, err)
			}))
			defer viewer.Close()

			raw, decoded, err := requestQuery(context.Background(), viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, string(raw), "9007199254740993")
			require.Len(t, decoded.Rows, 1)
			assert.Equal(t, json.Number("9007199254740993"), decoded.Rows[0][0])
		})
	}
}

func TestRequestQueryValidatesResultStructure(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		wantErr string
	}{
		{name: "null result", result: `null`, wantErr: "result must be an object"},
		{name: "missing columns", result: `{"rows":[],"truncated":false}`, wantErr: "missing columns"},
		{name: "missing rows", result: `{"columns":[],"truncated":false}`, wantErr: "missing rows"},
		{name: "missing truncated", result: `{"columns":[],"rows":[]}`, wantErr: "missing truncated"},
		{name: "columns are not an array", result: `{"columns":{},"rows":[],"truncated":false}`, wantErr: "decode columns"},
		{name: "column is not an object", result: `{"columns":[null],"rows":[],"truncated":false}`, wantErr: "columns[0]: must be an object"},
		{name: "column is missing type", result: `{"columns":[{"name":"n"}],"rows":[],"truncated":false}`, wantErr: "missing name or type"},
		{name: "rows are not an array", result: `{"columns":[],"rows":{},"truncated":false}`, wantErr: "decode rows"},
		{name: "row is not an array", result: `{"columns":[],"rows":[null],"truncated":false}`, wantErr: "rows[0]: must be an array"},
		{name: "row width does not match columns", result: `{"columns":[],"rows":[[1]],"truncated":false}`, wantErr: "got 1 values for 0 columns"},
		{name: "truncated is not a boolean", result: `{"columns":[],"rows":[],"truncated":"false"}`, wantErr: "decode truncated"},
		{name: "valid empty result with extra field", result: `{"columns":[],"rows":[],"truncated":false,"future":"accepted"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + test.result + `}`))
				require.NoError(t, err)
			}))
			defer viewer.Close()

			raw, result, err := requestQuery(context.Background(), viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.result, string(raw))
			assert.Empty(t, result.Columns)
			assert.Empty(t, result.Rows)
			assert.False(t, result.Truncated)
		})
	}
}

func TestRequestQueryPropagatesCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	viewer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseHandler
	}))
	defer viewer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := requestQuery(ctx, viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
		result <- err
	}()
	<-requestStarted
	cancel()
	err := <-result
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	close(releaseHandler)
}

func TestFormatQueryColumnsDistinguishesNullEmptyAndEscapesControls(t *testing.T) {
	result := queryResult{
		Columns: []queryColumn{{Name: "value"}, {Name: "payload"}},
		Rows: [][]any{
			{"", nil},
			{"e\u0301", map[string]any{"line": "one\ntwo"}},
		},
	}

	output := formatQueryColumns(result)
	assert.Equal(t, "value  payload            \n-----  -------------------\n       NULL               \né      {\"line\":\"one\\ntwo\"}\n", output)
	assert.False(t, strings.ContainsRune(output, '\x1b'))
}

func TestQueryDisplayWidthUsesTerminalCellWidths(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
	}{
		{name: "wide CJK cat", value: "猫", width: 2},
		{name: "wide CJK boundary", value: "界", width: 2},
		{name: "combining text", value: "e\u0301", width: 1},
		{name: "family emoji sequence", value: "👨‍👩‍👧‍👦", width: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.width, queryDisplayWidth(test.value))
		})
	}
}

func TestFormatQueryColumnsAlignsUnicodeTerminalCells(t *testing.T) {
	result := queryResult{
		Columns: []queryColumn{{Name: "text"}, {Name: "x"}},
		Rows: [][]any{
			{"猫", "x"},
			{"界界", "x"},
			{"e\u0301", "x"},
			{"👨‍👩‍👧‍👦", "x"},
		},
	}

	assert.Equal(t, "text  x\n----  -\n猫    x\n界界  x\né     x\n👨‍👩‍👧‍👦    x\n", formatQueryColumns(result))
}
