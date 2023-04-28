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
)

const (
	MAX_QUEUE_LENGTH = 10000
)

type desktopExporter struct {
	traceStore *TraceStore
	server     *Server
}

func (exporter *desktopExporter) pushMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	md := extractMetrics(metrics)
	for _, metric := range md {
		exporter.traceStore.AddMetric(ctx, metric)
	}
	return nil
}

func (exporter *desktopExporter) pushLogs(ctx context.Context, logs plog.Logs) error {
	ld := extractLogs(logs)
	for _, logData := range ld {
		exporter.traceStore.AddLog(ctx, logData)
	}
	return nil
}

func (exporter *desktopExporter) pushTraces(ctx context.Context, traces ptrace.Traces) error {
	spans := extractSpans(ctx, traces)
	for _, span := range spans {
		exporter.traceStore.Add(ctx, span)
	}
	return nil
}

func newDesktopExporter(cfg *Config) *desktopExporter {
	traceStore := NewTraceStore(MAX_QUEUE_LENGTH)
	server := NewServer(traceStore, cfg.Endpoint)
	return &desktopExporter{
		traceStore: traceStore,
		server:     server,
	}
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
