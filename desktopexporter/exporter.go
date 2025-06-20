package desktopexporter

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
)

type desktopExporter struct {
	server *server.Server
}

func newDesktopExporter(cfg *Config) *desktopExporter {
	server := server.NewServer(cfg.Endpoint, cfg.Db)
	return &desktopExporter{
		server: server,
	}
}

func (exporter *desktopExporter) pushTraces(ctx context.Context, source ptrace.Traces) error {
	spanDataSlice := traces.NewSpanPayload(source).ExtractSpans()
	exporter.server.Store.AddSpans(ctx, spanDataSlice)

	return nil
}

func (exporter *desktopExporter) pushMetrics(ctx context.Context, source pmetric.Metrics) error {
	metricsDataSlice := metrics.NewMetricsPayload(source).ExtractMetrics()
	for _, metricsData := range metricsDataSlice {
		fmt.Println(metricsData)
	}
	return nil
}

func (exporter *desktopExporter) pushLogs(ctx context.Context, source plog.Logs) error {
	logDataSlice := logs.NewLogsPayload(source).ExtractLogs()
	exporter.server.Store.AddLogs(ctx, logDataSlice)

	return nil
}

func (exporter *desktopExporter) Start(ctx context.Context, host component.Host) error {
	go func() {
		err := exporter.server.Start()

		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server: %s\n", err)
		}

	}()
	return nil
}

func (exporter *desktopExporter) Shutdown(ctx context.Context) error {
	return exporter.server.Close()
}
