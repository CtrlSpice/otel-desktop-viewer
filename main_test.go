package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	envprovider "go.opentelemetry.io/collector/confmap/provider/envprovider"
	yamlprovider "go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.opentelemetry.io/collector/otelcol"
)

func testOptions() configOptions {
	return configOptions{
		host:        "localhost",
		httpPort:    4318,
		grpcPort:    4317,
		browserPort: 8000,
		db:          "",
		dbMaxSize:   "",
	}
}

// resolveConfig runs the composed URIs through the same providers the binary
// uses and returns the fully validated collector config.
func resolveConfig(t *testing.T, o configOptions) (*otelcol.Config, error) {
	t.Helper()

	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: collectorURIs(o),
			ProviderFactories: []confmap.ProviderFactory{
				envprovider.NewFactory(),
				yamlprovider.NewFactory(),
			},
			DefaultScheme: "env",
		},
	})
	require.NoError(t, err)

	factories, err := components()
	require.NoError(t, err)

	return provider.Get(context.Background(), factories)
}

// validateServiceTelemetry unmarshals and validates the service::telemetry
// block specifically.
//
// It is separate from resolveConfig because otelcol.Config.Validate does not
// cover it: service::telemetry stays a raw confmap.Conf until the service
// builds the telemetry component at startup, so resolving and validating the
// collector config says nothing at all about the telemetry block.
//
// Mutation-checked, so its reach is known rather than assumed:
//
//	level: detailed -> level: bogus        caught
//	protocol: grpc  -> protocol: nonsense  NOT caught
//
// So this covers the block's shape and its enum values, but the declarative
// config does not validate exporter protocol or endpoint values at config time;
// a typo there surfaces when the collector starts, not here.
func validateServiceTelemetry(t *testing.T, o configOptions) error {
	t.Helper()

	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs: collectorURIs(o),
		ProviderFactories: []confmap.ProviderFactory{
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
		},
		DefaultScheme: "env",
	})
	require.NoError(t, err)

	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)

	sub, err := conf.Sub("service::telemetry")
	require.NoError(t, err)

	factories, err := components()
	require.NoError(t, err)

	telCfg := factories.Telemetry.CreateDefaultConfig()
	if err := sub.Unmarshal(telCfg); err != nil {
		return err
	}
	return xconfmap.Validate(telCfg)
}

func TestCollectorURIsResolve(t *testing.T) {
	t.Run("telemetry off", func(t *testing.T) {
		cfg, err := resolveConfig(t, testOptions())
		require.NoError(t, err)
		require.NoError(t, cfg.Validate())
	})

	t.Run("telemetry self", func(t *testing.T) {
		o := testOptions()
		o.selfTelemetry = true

		cfg, err := resolveConfig(t, o)
		require.NoError(t, err)
		require.NoError(t, cfg.Validate())
	})
}

// The composed service::telemetry block must unmarshal cleanly into
// otelconftelemetry's config. The bug this guards against is the two halves
// drifting apart -- the exporter asking to instrument itself while the service
// hands it noop providers, or vice versa -- and only this reaches the telemetry
// schema at all.
func TestServiceTelemetryValidates(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		require.NoError(t, validateServiceTelemetry(t, testOptions()))
	})

	t.Run("self", func(t *testing.T) {
		o := testOptions()
		o.selfTelemetry = true
		require.NoError(t, validateServiceTelemetry(t, o))
	})
}

// The exporter must actually be told to instrument itself. Composing the
// service::telemetry block alone would give the exporter working providers
// while leaving its own Telemetry field at the default, so it would emit
// nothing.
func TestSelfTelemetrySetsExporterMode(t *testing.T) {
	off := telemetryURIs(testOptions(), "localhost:4317")
	assert.NotContains(t, strings.Join(off, "\n"), "exporters::desktop::telemetry",
		"exporter telemetry should be left at its default when the flag is off")

	o := testOptions()
	o.selfTelemetry = true
	on := strings.Join(telemetryURIs(o, "localhost:4317"), "\n")

	assert.Contains(t, on, "exporters::desktop::telemetry: self")
	assert.Contains(t, on, "http://localhost:4317",
		"self telemetry should target this process's own OTLP receiver")
}

// Off must keep metrics at level none. Without it the collector's default
// config stands up a Pull (Prometheus) reader, so the default build would open
// a metrics endpoint nobody asked for.
func TestTelemetryOffKeepsMetricsNone(t *testing.T) {
	uris := telemetryURIs(testOptions(), "localhost:4317")
	require.Len(t, uris, 1)
	assert.Equal(t, `yaml:service::telemetry::metrics::level: none`, uris[0])
}

// The OTLP target must follow --grpc and --host rather than being hardcoded,
// or self-telemetry silently goes nowhere on a non-default port.
func TestSelfTelemetryFollowsGRPCEndpoint(t *testing.T) {
	o := testOptions()
	o.selfTelemetry = true
	o.grpcPort = 15317
	o.host = "127.0.0.1"

	joined := strings.Join(collectorURIs(o), "\n")
	assert.Contains(t, joined, "http://127.0.0.1:15317")
}
