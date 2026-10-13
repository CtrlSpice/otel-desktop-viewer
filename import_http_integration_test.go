package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

// File imports use the ordinary OTLP receiver, so the same JSON requests must
// reach the production store and be discoverable through its existing query API.
func TestFileImportThroughOTLPHTTP(t *testing.T) {
	viewer, traces, logs, metrics := startAttributeIntegration(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	factory := otlpreceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpreceiver.Config)
	cfg.Protocols.GRPC = configoptional.None[configgrpc.ServerConfig]()
	cfg.Protocols.HTTP.GetOrInsertDefault().ServerConfig.NetAddr.Endpoint = address
	set := receivertest.NewNopSettings(factory.Type())
	tr, err := factory.CreateTraces(t.Context(), set, cfg, traces)
	require.NoError(t, err)
	lr, err := factory.CreateLogs(t.Context(), set, cfg, logs)
	require.NoError(t, err)
	mr, err := factory.CreateMetrics(t.Context(), set, cfg, metrics)
	require.NoError(t, err)
	for _, receiver := range []component.Component{tr, lr, mr} {
		require.NoError(t, receiver.Start(t.Context(), componenttest.NewNopHost()))
		t.Cleanup(func() { require.NoError(t, receiver.Shutdown(context.Background())) })
	}
	requests := []struct{ signal, body, query string }{
		{"traces", `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"file-import","startTimeUnixNano":"1700000000000000000","endTimeUnixNano":"1700000000000000001"}]}]}]}`, "select count(*)::varchar from spans where name = 'file-import'"},
		{"logs", `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1700000000000000000","body":{"stringValue":"file-import"}}]}]}]}`, "select count(*)::varchar from logs"},
		{"metrics", `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"file-import","gauge":{"dataPoints":[{"timeUnixNano":"1700000000000000000","asInt":"9223372036854775807"}]}}]}]}]}`, "select count(*)::varchar from metrics where name = 'file-import'"},
	}
	for _, request := range requests {
		t.Run(request.signal, func(t *testing.T) {
			response, err := http.Post("http://"+address+"/v1/"+request.signal, "application/json", bytes.NewBufferString(request.body))
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode, string(body))
			_, result, err := requestQuery(t.Context(), http.DefaultClient, viewer, request.query, 1)
			require.NoError(t, err)
			require.Equal(t, "1", result.Rows[0][0])
		})
	}
}
