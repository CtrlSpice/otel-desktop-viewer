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
	defaultDb       = ""
)

// Creates a factory for the Desktop Exporter
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, metadata.TracesStability),
		exporter.WithMetrics(createMetricsExporter, metadata.MetricsStability),
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

// Create default configurations
func createDefaultConfig() component.Config {
	return &Config{
		Endpoint: defaultEndpoint,
		Db:       defaultDb,
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

	return exporterhelper.NewMetrics(
		ctx,
		set,
		desktopCfg,
		exporter.Unwrap().pushMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
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

	return exporterhelper.NewLogs(ctx, set, cfg,
		e.Unwrap().pushLogs,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
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

	return exporterhelper.NewTraces(ctx, set, cfg,
		e.Unwrap().pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithStart(e.Start),
		exporterhelper.WithShutdown(e.Shutdown),
	)
}

// This is the map of already created desktop exporters for particular configurations.
// We maintain this map because the Factory is asked trace, logs, and metric exporters separately
// when it gets CreateTracesExporter() and CreateMetricsExporter() but they must not
// create separate objects, they must use one desktopExporter object per configuration.
// When the exporter is shutdown it should be removed from this map so the same configuration
// can be recreated successfully.
var exporters = sharedcomponent.NewSharedComponents[*Config, *desktopExporter]()
