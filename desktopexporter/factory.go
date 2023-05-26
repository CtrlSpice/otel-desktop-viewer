package desktopexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/sharedcomponent"
)

const (
	typeStr   = "desktop"
	stability = component.StabilityLevelDevelopment
)

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeStr,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, stability),
		exporter.WithMetrics(createMetricsExporter, stability),
		exporter.WithLogs(createLogsExporter, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createMetricsExporter(ctx context.Context, set exporter.CreateSettings, config component.Config) (exporter.Metrics, error) {
	cfg := config.(*Config)
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	e, err := exporters.GetOrAdd(cfg, func() (*desktopExporter, error) {
		return newDesktopExporter(cfg), nil
	})
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewMetricsExporter(ctx, set, cfg,
		e.Unwrap().pushMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(e.Start),
	)
}

func createLogsExporter(ctx context.Context, set exporter.CreateSettings, config component.Config) (exporter.Logs, error) {
	cfg := config.(*Config)
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	e, err := exporters.GetOrAdd(cfg, func() (*desktopExporter, error) {
		return newDesktopExporter(cfg), nil
	})
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogsExporter(ctx, set, cfg,
		e.Unwrap().pushLogs,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(e.Start),
	)
}

func createTracesExporter(ctx context.Context, set exporter.CreateSettings, config component.Config) (exporter.Traces, error) {
	cfg := config.(*Config)
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	e, err := exporters.GetOrAdd(cfg, func() (*desktopExporter, error) {
		return newDesktopExporter(cfg), nil
	})
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewTracesExporter(ctx, set, cfg,
		e.Unwrap().pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(e.Start),
	)
}

// This is the map of already created desktop exporters for particular configurations.
// We maintain this map because the Factory is asked trace, logs, and metric exporters separately
// when it gets CreateTracesExporter() and CreateMetricsExporter() but they must not
// create separate objects, they must use one desktopExporter object per configuration.
// When the exporter is shutdown it should be removed from this map so the same configuration
// can be recreated successfully.
var exporters = sharedcomponent.NewSharedComponents[*Config, *desktopExporter]()
