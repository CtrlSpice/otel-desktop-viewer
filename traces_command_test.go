package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

type telemetryRoundTripFunc func(*http.Request) (*http.Response, error)

func (f telemetryRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestResolveTelemetrySearchWindowsAndErrors(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 30, 0, 123456789, time.FixedZone("offset", -7*60*60))
	query, err := resolveTelemetrySearch(telemetrySearchOptions{Since: time.Hour, Limit: 25}, false, now)
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(now.Add(-time.Hour).UnixNano(), 10), *query.StartTime)
	assert.Equal(t, strconv.FormatInt(now.UnixNano(), 10), *query.EndTime)

	query, err = resolveTelemetrySearch(telemetrySearchOptions{
		Since: time.Hour, Start: "2026-10-02T08:00:00.000000001Z",
		End: "2026-10-02T09:00:00.999999999+01:00", Limit: 7,
	}, false, now)
	require.NoError(t, err)
	assert.Equal(t, "1790928000000000001", *query.StartTime)
	assert.Equal(t, "1790928000999999999", *query.EndTime)

	for _, tc := range []struct {
		name         string
		options      telemetrySearchOptions
		sinceChanged bool
		message      string
	}{
		{"since conflict", telemetrySearchOptions{Since: time.Hour, Start: "2026-10-02T08:00:00Z", Limit: 25}, true, "cannot be combined"},
		{"zero since", telemetrySearchOptions{Limit: 25}, false, "greater than zero"},
		{"zero limit", telemetrySearchOptions{Since: time.Hour}, false, "greater than zero"},
		{"bad start", telemetrySearchOptions{Since: time.Hour, Start: "yesterday", Limit: 25}, false, "use RFC3339"},
		{"reverse range", telemetrySearchOptions{Since: time.Hour, Start: "2026-10-02T09:00:00Z", End: "2026-10-02T08:00:00Z", Limit: 25}, false, "must not be after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveTelemetrySearch(tc.options, tc.sinceChanged, now)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestTracesCommandPreservesSummaryFieldsInTableAndJSON(t *testing.T) {
	const summary = `{"traceID":"0123456789abcdef0123456789abcdef","hasRootSpan":true,"rootSpan":{"serviceName":"checkout","name":"POST /checkout"},"startTime":"1790928000123456789","durationNs":"9007199254740993","spanCount":4,"errorCount":1}`
	const matchedSpans = `[{"traceID":"0123456789abcdef0123456789abcdef","spanID":"00000000000000a1"},{"traceID":"0123456789abcdef0123456789abcdef","spanID":"00000000000000a2"}]`
	const unfilteredResponse = `[` + summary + `]`
	filteredResponse := `[` + summary[:len(summary)-1] + `,"matchedSpans":` + matchedSpans + `}]`
	requests := make(chan queryRPCRequest, 3)
	viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpcRequest queryRPCRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&rpcRequest))
		requests <- rpcRequest
		response := unfilteredResponse
		if rpcRequest.Params["query"] != nil {
			response = filteredResponse
		}
		_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + response + `}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	fixedNow := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	cmd := newTracesCommand(http.DefaultClient, func() time.Time { return fixedNow })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--endpoint", viewer.URL, "--service", "checkout", "--limit", "25"})
	require.NoError(t, cmd.Execute())
	const tableOutput = "traceID                           hasRootSpan  rootSpan                                            startTime            durationNs        spanCount  errorCount  matchedSpans                                                                                                                                           \n" +
		"--------------------------------  -----------  --------------------------------------------------  -------------------  ----------------  ---------  ----------  -------------------------------------------------------------------------------------------------------------------------------------------------------\n" +
		"0123456789abcdef0123456789abcdef  true         {\"name\":\"POST /checkout\",\"serviceName\":\"checkout\"}  1790928000123456789  9007199254740993  4          1           [{\"spanID\":\"00000000000000a1\",\"traceID\":\"0123456789abcdef0123456789abcdef\"},{\"spanID\":\"00000000000000a2\",\"traceID\":\"0123456789abcdef0123456789abcdef\"}]\n"
	assert.Equal(t, tableOutput, output.String())

	request := <-requests
	assert.Equal(t, "searchTraces", request.Method)
	assert.Equal(t, float64(26), request.Params["limit"])
	assert.Equal(t, strconv.FormatInt(fixedNow.Add(-time.Hour).UnixNano(), 10), request.Params["startTime"])
	assert.Equal(t, strconv.FormatInt(fixedNow.UnixNano(), 10), request.Params["endTime"])
	condition := request.Params["query"].(map[string]any)["query"].(map[string]any)
	assert.Equal(t, "checkout", condition["value"])

	output.Reset()
	cmd = newTracesCommand(http.DefaultClient, func() time.Time { return fixedNow })
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--endpoint", viewer.URL, "--service", "checkout", "--json"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, filteredResponse+"\n", output.String())
	<-requests

	output.Reset()
	cmd = newTracesCommand(http.DefaultClient, func() time.Time { return fixedNow })
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--endpoint", viewer.URL, "--json"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, unfilteredResponse+"\n", output.String())
	<-requests
}

func TestTracesCommandTruncationEmptyErrorsCancellationAndHelp(t *testing.T) {
	t.Run("filtered JSON lookahead", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[` +
				`{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"1","spanID":"0000000000000001"}]},` +
				`{"traceID":"2","hasRootSpan":false,"rootSpan":null,"startTime":"2","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"2","spanID":"0000000000000002"}]}]}`))
		}))
		defer viewer.Close()
		cmd := newTracesCommand(http.DefaultClient, time.Now)
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs([]string{"--endpoint", viewer.URL, "--service", "checkout", "--limit", "1", "--json"})
		require.NoError(t, cmd.Execute())
		assert.Equal(t, `[{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"1","spanID":"0000000000000001"}]}]`+"\n", output.String())
	})

	t.Run("table truncation", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[` +
				`{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0},` +
				`{"traceID":"2","hasRootSpan":false,"rootSpan":null,"startTime":"2","durationNs":null,"spanCount":1,"errorCount":0}]}`))
		}))
		defer viewer.Close()
		cmd := newTracesCommand(http.DefaultClient, time.Now)
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs([]string{"--endpoint", viewer.URL, "--limit", "1"})
		require.NoError(t, cmd.Execute())
		assert.Contains(t, output.String(), "[1 rows shown; more rows available; use --limit to return more]")
		assert.NotContains(t, output.String(), "  2 ")
	})

	t.Run("empty", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[]}`))
		}))
		defer viewer.Close()
		result, err := requestTelemetrySearch(context.Background(), viewer.Client(), viewer.URL, "searchTraces", telemetrySearchQuery{Limit: 25}, traceSummaryFields)
		require.NoError(t, err)
		assert.Empty(t, result.Rows)
		assert.NotNil(t, result.Summaries)
	})

	t.Run("RPC and transport errors", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid query"}}`))
		}))
		defer viewer.Close()
		_, err := requestTelemetrySearch(context.Background(), viewer.Client(), viewer.URL, "searchTraces", telemetrySearchQuery{Limit: 25}, traceSummaryFields)
		require.ErrorContains(t, err, "viewer searchTraces error -32602")

		client := &http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		})}
		_, err = requestTelemetrySearch(context.Background(), client, "http://viewer.test", "searchTraces", telemetrySearchQuery{Limit: 25}, traceSummaryFields)
		require.ErrorContains(t, err, "connection refused")
	})

	t.Run("cancellation", func(t *testing.T) {
		client := &http.Client{Transport: telemetryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := requestTelemetrySearch(ctx, client, "http://viewer.test", "searchTraces", telemetrySearchQuery{Limit: 25}, traceSummaryFields)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("offline help", func(t *testing.T) {
		root := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs([]string{"traces", "--help", "--endpoint", "http://127.0.0.1:1"})
		require.NoError(t, root.Execute())
		for _, text := range []string{"🧵", "--service", "--since", "default 1h0m0s", "--start", "--end", "--limit", "default 25", "--endpoint", "--json", "checkout"} {
			assert.Contains(t, output.String(), text)
		}
	})
}

func TestRequestTelemetrySearchRequiresOneCompleteJSONRPCResponse(t *testing.T) {
	const summary = `{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1790928000123456789","durationNs":9007199254740993,"spanCount":1,"errorCount":0}`
	validResponse := `{"jsonrpc":"2.0","id":1,"result":[` + summary + `]}`

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

			result, err := requestTelemetrySearch(
				context.Background(), viewer.Client(), viewer.URL, "searchTraces",
				telemetrySearchQuery{Limit: 25}, traceSummaryFields,
			)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, result.Summaries, 1)
			assert.Equal(t, summary, string(result.Summaries[0]))
			require.Len(t, result.Rows, 1)
			assert.Equal(t, json.Number("9007199254740993"), result.Rows[0][4])
		})
	}
}

func TestRequestTelemetrySearchRequiresMatchingJSONRPCIdentity(t *testing.T) {
	const result = `"result":[{"value":9007199254740993,"nullable":null}]`
	methods := []string{"searchTraces", "searchLogs", "searchMetricSummaries"}
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "valid response", response: `{"jsonrpc":"2.0","id":1,` + result + `}`},
		{name: "missing version", response: `{"id":1,` + result + `}`, wantErr: "invalid jsonrpc version"},
		{name: "wrong version", response: `{"jsonrpc":"1.0","id":1,` + result + `}`, wantErr: "invalid jsonrpc version"},
		{name: "null version", response: `{"jsonrpc":null,"id":1,` + result + `}`, wantErr: "invalid jsonrpc version"},
		{name: "missing id", response: `{"jsonrpc":"2.0",` + result + `}`, wantErr: "response id does not match request id"},
		{name: "wrong numeric id", response: `{"jsonrpc":"2.0","id":2,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "null id", response: `{"jsonrpc":"2.0","id":null,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "string id", response: `{"jsonrpc":"2.0","id":"1",` + result + `}`, wantErr: "response id does not match request id"},
		{name: "decimal id", response: `{"jsonrpc":"2.0","id":1.0,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "exponent id", response: `{"jsonrpc":"2.0","id":1e0,` + result + `}`, wantErr: "response id does not match request id"},
		{name: "valid error response", response: `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"query rejected"}}`, wantErr: "error -32602: query rejected"},
		{name: "empty response", wantErr: "decode viewer response: EOF"},
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
						_, err := writer.Write([]byte(test.response))
						require.NoError(t, err)
					}))
					defer viewer.Close()

					decoded, err := requestTelemetrySearch(
						context.Background(), viewer.Client(), viewer.URL, method,
						telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"},
					)
					if test.wantErr != "" {
						require.Error(t, err)
						assert.Contains(t, err.Error(), test.wantErr)
						return
					}

					require.NoError(t, err)
					require.Len(t, decoded.Summaries, 1)
					assert.Equal(t, `{"value":9007199254740993,"nullable":null}`, string(decoded.Summaries[0]))
					require.Len(t, decoded.Rows, 1)
					assert.Equal(t, json.Number("9007199254740993"), decoded.Rows[0][0])
					assert.Nil(t, decoded.Rows[0][1])
				})
			}
		})
	}
}
