package desktopexporter

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/metadata"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter"
)

func testExporterSettings(t *testing.T) exporter.Settings {
	t.Helper()
	return exporter.Settings{
		ID:                component.NewID(metadata.Type),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestCreateExporterNilConfig(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)

	_, err := createMetricsExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")

	_, err = createLogsExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")

	_, err = createTracesExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")
}

func TestSharedExporterSingleton(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	cfg := &Config{Endpoint: freeLocalAddr(t), DbMaxSize: "0"}

	mExp, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)
	lExp, err := createLogsExporter(ctx, set, cfg)
	require.NoError(t, err)
	tExp, err := createTracesExporter(ctx, set, cfg)
	require.NoError(t, err)

	require.NoError(t, mExp.Start(ctx, componenttest.NewNopHost()))
	require.NoError(t, lExp.Start(ctx, componenttest.NewNopHost()))
	require.NoError(t, tExp.Start(ctx, componenttest.NewNopHost()))

	resp, err := http.Get("http://" + cfg.Endpoint + "/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, tExp.Shutdown(ctx))
	require.NoError(t, lExp.Shutdown(ctx))
	require.NoError(t, mExp.Shutdown(ctx))
}

func TestFactoryShutdownAndRestart(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	cfg := &Config{Endpoint: freeLocalAddr(t), DbMaxSize: "0"}

	exp, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))

	resp, err := http.Get("http://" + cfg.Endpoint + "/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, exp.Shutdown(ctx))

	exp2, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp2.Start(ctx, componenttest.NewNopHost()))
	require.NoError(t, exp2.Shutdown(ctx))
}

func TestFactoryRecreateAfterShutdown(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	cfg := &Config{Endpoint: freeLocalAddr(t), DbMaxSize: "0"}

	mExp, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, mExp.Start(ctx, componenttest.NewNopHost()))
	require.NoError(t, mExp.Shutdown(ctx))

	lExp, err := createLogsExporter(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, lExp.Start(ctx, componenttest.NewNopHost()))

	resp, err := http.Get("http://" + cfg.Endpoint + "/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, lExp.Shutdown(ctx))
}
