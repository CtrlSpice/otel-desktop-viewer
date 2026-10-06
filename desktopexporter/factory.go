package desktopexporter

import (
	"context"
	"errors"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// NewFactory creates a factory for the desktop exporter.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, metadata.TracesStability),
		exporter.WithMetrics(createMetricsExporter, metadata.MetricsStability),
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

// createDefaultConfig returns the default exporter configuration.
func createDefaultConfig() component.Config {
	return &Config{
		Telemetry:    TelemetryDisabled,
		SendingQueue: defaultSendingQueue(),
	}
}

// newSignalExporter validates config and builds one signal pipeline's
// write-only exporter. Start resolves the store from the DuckDB extension.
func newSignalExporter(config component.Config, set exporter.Settings) (*desktopExporter, *Config, error) {
	if config == nil {
		return nil, nil, errors.New("nil config")
	}

	cfg := config.(*Config)
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	e, err := newDesktopExporter(cfg, set.TelemetrySettings)
	if err != nil {
		return nil, nil, err
	}
	return e, cfg, nil
}

func createMetricsExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Metrics, error) {
	e, cfg, err := newSignalExporter(config, set)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewMetrics(
		ctx,
		set,
		cfg,
		e.pushMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithQueue(cfg.SendingQueue),
		exporterhelper.WithStart(e.Start),
	)
}

func createLogsExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Logs, error) {
	e, cfg, err := newSignalExporter(config, set)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogs(ctx, set, cfg,
		e.pushLogs,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithQueue(cfg.SendingQueue),
		exporterhelper.WithStart(e.Start),
	)
}

func createTracesExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Traces, error) {
	e, cfg, err := newSignalExporter(config, set)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewTraces(ctx, set, cfg,
		e.pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithQueue(cfg.SendingQueue),
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithStart(e.Start),
	)
}
