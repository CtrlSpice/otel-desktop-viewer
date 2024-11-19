package desktopexporter

import (
	"context"
	"errors"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/metadata"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/sharedcomponent"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	defaultEndpoint = "localhost:8000"
)

// Creates a factory for the Desktop Exporter
func NewFactory(dbFilename string) exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig(dbFilename),
		exporter.WithTraces(createTracesExporter, metadata.TracesStability),
		exporter.WithMetrics(createMetricsExporter, metadata.MetricsStability),
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

// Create default configurations
func createDefaultConfig(dbFilename string) func() component.Config {
	return func() component.Config {
		return &Config{
			Endpoint:   defaultEndpoint,
			DBFilename: dbFilename,
		}
	}
}

func createMetricsExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Metrics, error) {
	if config == nil {
		return nil, errors.New("nil config")
	}

	desktopCfg := config.(*Config)
	err := desktopCfg.Validate()
	if err != nil {
		return nil, err
	}

	exporter, err := exporters.GetOrAdd(desktopCfg, func() (*desktopExporter, error) {
		return newDesktopExporter(desktopCfg), nil
	})
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewMetricsExporter(
		ctx,
		set,
		desktopCfg,
		exporter.Unwrap().pushMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(exporter.Start),
	)
}

func createLogsExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Logs, error) {
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
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(e.Start),
	)
}

func createTracesExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Traces, error) {
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
