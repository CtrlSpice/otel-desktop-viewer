package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/extension/extensiontest"
)

type attributeIntegrationHost struct {
	ext component.Component
}

func (h attributeIntegrationHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{component.NewID(duckdbextension.Type): h.ext}
}

// Each collector integration test owns its extension and in-memory store.
func startAttributeIntegration(t *testing.T) (string, exporter.Traces, exporter.Logs, exporter.Metrics) {
	t.Helper()
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	factory := duckdbextension.NewFactory()
	cfg := factory.CreateDefaultConfig().(*duckdbextension.Config)
	cfg.Endpoint = address
	cfg.DbMaxSize = "0"
	ext, err := factory.Create(ctx, extensiontest.NewNopSettings(duckdbextension.Type), cfg)
	require.NoError(t, err)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, ext.Shutdown(context.Background())) })

	exporterFactory := desktopexporter.NewFactory()
	exporterCfg := exporterFactory.CreateDefaultConfig().(*desktopexporter.Config)
	exporterCfg.SendingQueue = configoptional.None[exporterhelper.QueueBatchConfig]()
	tracesExporter, err := exporterFactory.CreateTraces(ctx, exporter.Settings{
		ID: component.NewID(exporterFactory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, tracesExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, tracesExporter.Shutdown(context.Background())) })
	settings := exporter.Settings{ID: component.NewID(exporterFactory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	logsExporter, err := exporterFactory.CreateLogs(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, logsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, logsExporter.Shutdown(context.Background())) })
	metricsExporter, err := exporterFactory.CreateMetrics(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, metricsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, metricsExporter.Shutdown(context.Background())) })
	return "http://" + address, tracesExporter, logsExporter, metricsExporter
}

// Query the production RPC endpoint without depending on CLI implementation.
func requestQuery(ctx context.Context, client *http.Client, endpoint, statement string, limit uint64) (json.RawMessage, struct{ Rows [][]any }, error) {
	var result struct{ Rows [][]any }
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "query",
		"params": map[string]any{"sql": statement, "limit": limit},
	})
	if err != nil {
		return nil, result, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, result, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, result, fmt.Errorf("query returned HTTP %s", response.Status)
	}
	var rpc struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
		return nil, result, err
	}
	if rpc.JSONRPC != "2.0" || rpc.ID != 1 || (len(rpc.Error) != 0 && string(rpc.Error) != "null") {
		return nil, result, fmt.Errorf("invalid query response: %s", rpc.Error)
	}
	err = json.Unmarshal(rpc.Result, &result)
	return rpc.Result, result, err
}
