package duckdbextension

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Type is the component type of the extension, as referenced from collector
// config (`extensions: duckdb:` / `service::extensions: [duckdb]`).
var Type = component.MustNewType("duckdb")

const (
	defaultEndpoint = "localhost:8000"
	defaultDb       = ""
)

// NewFactory creates a factory for the DuckDB store extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		Type,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelDevelopment,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:  defaultEndpoint,
		Db:        defaultDb,
		Telemetry: TelemetryDisabled,
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	if cfg == nil {
		return nil, errors.New("nil config")
	}
	extCfg := cfg.(*Config)
	if err := extCfg.Validate(); err != nil {
		return nil, err
	}
	return newDuckDBExtension(extCfg, set.TelemetrySettings)
}
