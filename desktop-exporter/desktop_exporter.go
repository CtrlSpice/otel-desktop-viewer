package desktopexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type desktopExporter struct {
	logger          *zap.Logger
	traceStore      *TraceStore
	tracesMarshaler ptrace.Marshaler
}

func (exporter *desktopExporter) pushTraces(ctx context.Context, traces ptrace.Traces) error {
	spans := extractSpans(ctx, traces)
	for _, span := range spans {
		exporter.traceStore.Add(ctx, span)
	}

	return nil
}

func newDesktopExporter(logger *zap.Logger) *desktopExporter {

	return &desktopExporter{
		logger:          logger,
		traceStore:      NewTraceStore(),
		tracesMarshaler: ptrace.NewJSONMarshaler(),
	}
}

func (exporter *desktopExporter) Start(_ context.Context, host component.Host) error {

	return nil
}

func (exporter *desktopExporter) Shutdown(_ context.Context) error {

	return nil
}
