package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

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
