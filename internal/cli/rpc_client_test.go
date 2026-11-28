package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

func TestRequestViewerRPCFramingAndTrailingBytes(t *testing.T) {
	const resultPayload = `{"value":9007199254740993,"nullable":null}`
	validResponse := `{"jsonrpc":"2.0","id":1,"result":` + resultPayload + `}`

	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "ordinary response", response: validResponse},
		{name: "trailing whitespace", response: validResponse + " \n\t\r"},
		{name: "malformed trailing data", response: validResponse + ` trailing`, wantErr: "decode viewer response: trailing data"},
		{name: "second JSON value", response: validResponse + ` {"jsonrpc":"2.0"}`, wantErr: "decode viewer response: additional JSON value"},
		{name: "empty response", response: "", wantErr: "decode viewer response: EOF"},
		{name: "missing result", response: `{"jsonrpc":"2.0","id":1}`, wantErr: "decode viewer response: missing result"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viewer := staticRPCServer(t, tc.response)
			raw, err := requestViewerRPC(context.Background(), viewer.Client(), viewer.URL, "query", nil)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, resultPayload, string(raw))
		})
	}
}

func TestRequestViewerRPCVersionAndIDValidation(t *testing.T) {
	const result = `"result":[{"value":9007199254740993,"nullable":null}]`
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
		{name: "valid error response", response: `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"query rejected"}}`, wantErr: "viewer query error -32602: query rejected"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viewer := staticRPCServer(t, tc.response)
			raw, err := requestViewerRPC(context.Background(), viewer.Client(), viewer.URL, "query", nil)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, `[{"value":9007199254740993,"nullable":null}]`, string(raw))
		})
	}
}

func TestRequestViewerRPCErrorsAndCancellation(t *testing.T) {
	t.Run("invalid endpoint", func(t *testing.T) {
		_, err := requestViewerRPC(context.Background(), offlineHTTPClient(t), "not-a-url", "query", nil)
		require.ErrorContains(t, err, "invalid viewer endpoint")
	})

	t.Run("HTTP status error", func(t *testing.T) {
		viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer viewer.Close()
		_, err := requestViewerRPC(context.Background(), viewer.Client(), viewer.URL, "query", nil)
		require.ErrorContains(t, err, "HTTP 503 Service Unavailable")
	})

	t.Run("RPC error", func(t *testing.T) {
		viewer := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid query"}}`)
		_, err := requestViewerRPC(context.Background(), viewer.Client(), viewer.URL, "searchTraceSummaries", nil)
		require.ErrorContains(t, err, "viewer searchTraceSummaries error -32602: Invalid query")
	})

	t.Run("transport errors", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
			want string
		}{
			{"connection refused", errors.New("connection refused"), "connection refused"},
			{"deadline exceeded", context.DeadlineExceeded, context.DeadlineExceeded.Error()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, tc.err
				})}
				_, err := requestViewerRPC(context.Background(), client, "http://viewer.test", "query", nil)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
				if errors.Is(tc.err, context.DeadlineExceeded) {
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				}
			})
		}
	})

	t.Run("pre-cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := requestViewerRPC(ctx, canceledHTTPClient(), "http://viewer.test", "query", nil)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("mid-flight cancellation", func(t *testing.T) {
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
	})
}

func TestRPCConsumersPreserveFidelityAndRejectInvalidEnvelopes(t *testing.T) {
	t.Run("search consumers preserve 64-bit numbers and enforce RPC framing", func(t *testing.T) {
		const item = `{"value":9007199254740993,"nullable":null}`
		valid := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":[`+item+`]}`)
		badID := staticRPCServer(t, `{"jsonrpc":"2.0","id":2,"result":[`+item+`]}`)
		trailing := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":[`+item+`]} {}`)
		rpcErr := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid query"}}`)

		for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
			t.Run(method, func(t *testing.T) {
				decoded, err := requestTelemetrySearch(context.Background(), valid.Client(), valid.URL, method, telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"})
				require.NoError(t, err)
				require.Len(t, decoded.Summaries, 1)
				assert.Equal(t, item, string(decoded.Summaries[0]))
				require.Len(t, decoded.Rows, 1)
				assert.Equal(t, json.Number("9007199254740993"), decoded.Rows[0][0])
				assert.Nil(t, decoded.Rows[0][1])

				_, err = requestTelemetrySearch(context.Background(), badID.Client(), badID.URL, method, telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"})
				require.ErrorContains(t, err, "response id does not match request id")

				_, err = requestTelemetrySearch(context.Background(), trailing.Client(), trailing.URL, method, telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"})
				require.ErrorContains(t, err, "additional JSON value")

				_, err = requestTelemetrySearch(context.Background(), rpcErr.Client(), rpcErr.URL, method, telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"})
				require.ErrorContains(t, err, "viewer "+method+" error -32602")

				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_, err = requestTelemetrySearch(ctx, canceledHTTPClient(), "http://viewer.test", method, telemetrySearchQuery{Limit: 25}, []string{"value", "nullable"})
				require.ErrorIs(t, err, context.Canceled)
			})
		}
	})

	t.Run("query consumer preserves exact numbers and rejects invalid envelopes", func(t *testing.T) {
		const payload = `{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[9007199254740993]],"truncated":false}`
		valid := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+payload+`}`)
		raw, decoded, err := requestQuery(context.Background(), valid.Client(), valid.URL, "select 1", queryDefaultLimit)
		require.NoError(t, err)
		assert.Equal(t, payload, string(raw))
		require.Len(t, decoded.Rows, 1)
		assert.Equal(t, json.Number("9007199254740993"), decoded.Rows[0][0])

		trailing := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+payload+`} {}`)
		_, _, err = requestQuery(context.Background(), trailing.Client(), trailing.URL, "select 1", queryDefaultLimit)
		require.ErrorContains(t, err, "additional JSON value")

		httpErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer httpErr.Close()
		_, _, err = requestQuery(context.Background(), httpErr.Client(), httpErr.URL, "select 1", queryDefaultLimit)
		require.ErrorContains(t, err, "HTTP 503 Service Unavailable")
	})

	t.Run("span consumer rejects trailing JSON", func(t *testing.T) {
		trailing := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":{"status":"notFound","spanID":"000000000000002a","traceID":null}} {}`)
		_, _, err := execCLI(t, cliOptions{client: trailing.Client()}, "span", "000000000000002a", "--endpoint", trailing.URL)
		require.ErrorContains(t, err, "additional JSON value")
	})
}

func TestRPCPropagatesToolContext(t *testing.T) {
	const inherited = "00-11111111111111111111111111111111-2222222222222222-01"
	const explicit = "00-33333333333333333333333333333333-4444444444444444-01"
	for _, tc := range []struct{ name, env, parent, want string }{
		{"tool environment", inherited, "", inherited},
		{"explicit context wins", inherited, explicit, explicit},
		{"invalid environment ignored", "not-a-traceparent", "", ""},
		{"zero IDs ignored", "00-00000000000000000000000000000000-0000000000000000-01", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRACEPARENT", tc.env)
			t.Setenv("TRACESTATE", "vendor=state")
			var got string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("traceparent")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			}))
			defer server.Close()
			ctx := (propagation.TraceContext{}).Extract(context.Background(), propagation.MapCarrier{"traceparent": tc.parent})
			_, err := requestViewerRPC(ctx, server.Client(), server.URL, "getStats", nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
