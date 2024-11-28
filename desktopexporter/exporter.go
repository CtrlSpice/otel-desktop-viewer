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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

type desktopExporter struct {
	server *server.Server
}

func newDesktopExporter(cfg *Config) *desktopExporter {
	server := server.NewServer(cfg.Endpoint, cfg.DbPath, cfg.IsDev)
	return &desktopExporter{
		server: server,
	}
}

func (exporter *desktopExporter) pushTraces(ctx context.Context, traces ptrace.Traces) error {
	spanDataSlice := telemetry.NewSpanPayload(traces).ExtractSpans()
	exporter.server.Store.AddSpans(ctx, spanDataSlice)

	return nil
}

func (exporter *desktopExporter) pushMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	metricsDataSlice := telemetry.NewMetricsPayload(metrics).ExtractMetrics()
	for _, metricsData := range metricsDataSlice {
		fmt.Println(metricsData.ID())
	}
	return nil
}

func (exporter *desktopExporter) pushLogs(ctx context.Context, logs plog.Logs) error {
	logDataSlice := telemetry.NewLogsPayload(logs).ExtractLogs()
	for _, logData := range logDataSlice {
		fmt.Println(logData.ID())
	}
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
