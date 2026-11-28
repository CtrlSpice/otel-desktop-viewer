package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryCommandPrintsColumnsAndSendsSQL(t *testing.T) {
	var request queryRPCRequest
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rpc", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"columns":[{"name":"service","type":"VARCHAR"},{"name":"count","type":"BIGINT"}],"rows":[["checkout",9007199254740993],[null,0]],"truncated":true}}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	out, _, err := execCLI(t, cliOptions{client: viewer.Client(), mergeErr: true},
		"query", "select service_name, count(*) from spans group by service_name", "--endpoint", viewer.URL, "--limit", "2")
	require.NoError(t, err)
	assert.Equal(t, "service   count           \n--------  ----------------\ncheckout  9007199254740993\nNULL      0               \n[2 rows shown; more rows available; use --limit to return more]\n", out)
	assert.Equal(t, "query", request.Method)
	assert.Equal(t, "select service_name, count(*) from spans group by service_name", request.Params["sql"])
	assert.Equal(t, float64(2), request.Params["limit"])
}

func TestQueryCommandJSONPreservesIntegerTokensNullAndEmptyText(t *testing.T) {
	const result = `{"columns":[{"name":"n","type":"BIGINT"},{"name":"text","type":"VARCHAR"}],"rows":[[9007199254740993,""],[0,null]],"truncated":false}`
	viewer := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+result+`}`)

	out, _, err := execCLI(t, cliOptions{client: viewer.Client(), mergeErr: true},
		"query", "select n, text from values", "--endpoint", viewer.URL, "--json")
	require.NoError(t, err)
	assert.Equal(t, result+"\n", out)
}

func TestQueryCommandDefaultLimitAndOfflineHelp(t *testing.T) {
	requestReceived := make(chan queryRPCRequest, 1)
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request queryRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requestReceived <- request
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"columns":[],"rows":[],"truncated":false}}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	_, _, err := execCLI(t, cliOptions{client: viewer.Client()}, "query", "select 1", "--endpoint", viewer.URL)
	require.NoError(t, err)
	request := <-requestReceived
	assert.Equal(t, float64(queryDefaultLimit), request.Params["limit"])

	out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t), mergeErr: true},
		"query", "--help", "--endpoint", "http://127.0.0.1:1")
	require.NoError(t, err)
	assertGolden(t, "help_query", out)

	out, _, err = execCLI(t, cliOptions{client: offlineHTTPClient(t), mergeErr: true}, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "query       🦆 Run SQL against a running viewer")
}

func TestQueryCommandReturnsRPCFailureWithoutUsage(t *testing.T) {
	viewer := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"query rejected: write statement"}}`)
	out, _, err := execCLI(t, cliOptions{client: viewer.Client(), mergeErr: true},
		"query", "delete from spans", "--endpoint", viewer.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "viewer query error -32602: query rejected: write statement")
	assert.NotContains(t, out, "Usage:")
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viewer := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+tc.result+`}`)
			raw, result, err := requestQuery(context.Background(), viewer.Client(), viewer.URL, "select 1", queryDefaultLimit)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.result, string(raw))
			assert.Empty(t, result.Columns)
			assert.Empty(t, result.Rows)
			assert.False(t, result.Truncated)
		})
	}
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.width, queryDisplayWidth(tc.value))
		})
	}
}

func TestFormatQueryColumnsAlignsUnicodeTerminalCellsGolden(t *testing.T) {
	result := queryResult{
		Columns: []queryColumn{{Name: "text"}, {Name: "x"}},
		Rows: [][]any{
			{"猫", "x"},
			{"界界", "x"},
			{"e\u0301", "x"},
			{"👨‍👩‍👧‍👦", "x"},
		},
	}

	assertGolden(t, "query_table_unicode", formatQueryColumns(result))
}
