package server

import (
	"context"
	"database/sql/driver"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/redactionprocessor"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.yaml.in/yaml/v3"
)

func enabledEvalFlags(t *testing.T) *features.Flags {
	t.Helper()
	file := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0o600))
	flags, err := features.New(t.Context(), file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, flags.Shutdown(context.Background())) })
	return flags
}

// Exercise the real HTTP receiver and pinned upstream redactor using the shipped
// policy. The final consumer is the same retained log ingest used by the desktop
// exporter; no upload path receives a store-writing test hook.
func startEvalReceiver(t *testing.T, s *Server) {
	t.Helper()
	sink, err := consumer.NewLogs(func(ctx context.Context, data plog.Logs) error {
		return s.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data, s.store.FlushedIDs()) })
	})
	require.NoError(t, err)
	policy, err := os.ReadFile("../../../collector_redaction.yaml")
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(policy, &raw))
	factory := redactionprocessor.NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NoError(t, confmap.NewFromStringMap(raw["processors"].(map[string]any)["redaction"].(map[string]any)).Unmarshal(cfg))
	p, err := factory.CreateLogs(t.Context(), processor.Settings{ID: component.MustNewID("redaction"), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, cfg, sink)
	require.NoError(t, err)
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := listener.Addr().String()
	require.NoError(t, listener.Close())
	rf := otlpreceiver.NewFactory()
	rc := rf.CreateDefaultConfig()
	require.NoError(t, confmap.NewFromStringMap(map[string]any{"protocols": map[string]any{"http": map[string]any{"endpoint": endpoint}}}).Unmarshal(rc))
	r, err := rf.CreateLogs(t.Context(), receiver.Settings{ID: component.MustNewID("otlp"), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, rc, p)
	require.NoError(t, err)
	require.NoError(t, r.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, r.Shutdown(context.Background())) })
	s.evalReceiverURL = "http://" + endpoint + "/v1/logs"
}
