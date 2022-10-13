package desktopexporter

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	typeStr                   = "desktop"
	defaultSamplingInitial    = 2
	defaultSamplingThereafter = 500
)

func NewFactory() component.ExporterFactory {
	return component.NewExporterFactory(
		typeStr,
		createDefaultConfig,
		component.WithTracesExporter(createTracesExporter, component.StabilityLevelInDevelopment),
	)
}

func createDefaultConfig() config.Exporter {
	return &Config{
		ExporterSettings:   config.NewExporterSettings(config.NewComponentID(typeStr)),
		LogLevel:           zapcore.InfoLevel,
		SamplingInitial:    defaultSamplingInitial,
		SamplingThereafter: defaultSamplingThereafter,
	}
}

func createTracesExporter(ctx context.Context, set component.ExporterCreateSettings, config config.Exporter) (component.TracesExporter, error) {
	cfg := config.(*Config)
	exporterLogger := createLogger(cfg, set.TelemetrySettings.Logger)
	desktopExporter := newDesktopExporter(exporterLogger)
	return exporterhelper.NewTracesExporter(ctx, set, cfg,
		desktopExporter.pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithStart(desktopExporter.Start),
		exporterhelper.WithShutdown(desktopExporter.Shutdown),
	)
}

func createLogger(cfg *Config, logger *zap.Logger) *zap.Logger {
	core := zapcore.NewSamplerWithOptions(
		logger.Core(),
		1*time.Second,
		cfg.SamplingInitial,
		cfg.SamplingThereafter,
	)

	return zap.New(core)
}
