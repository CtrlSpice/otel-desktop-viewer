package collector

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	envprovider "go.opentelemetry.io/collector/confmap/provider/envprovider"
	yamlprovider "go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

func testOptions() ConfigOptions {
	return ConfigOptions{
		Host:        "localhost",
		HTTPPort:    4318,
		GRPCPort:    4317,
		BrowserPort: 8000,
		DB:          "",
		DBMaxSize:   "",
	}
}

// resolveConfig runs the composed URIs through the same providers the binary
// uses and returns the fully validated collector config.
func resolveConfig(t *testing.T, o ConfigOptions) (*otelcol.Config, error) {
	t.Helper()

	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: URIs(o),
			ProviderFactories: []confmap.ProviderFactory{
				envprovider.NewFactory(),
				yamlprovider.NewFactory(),
			},
			DefaultScheme: "env",
		},
	})
	require.NoError(t, err)

	factories, err := Components()
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
// This validates the block's shape and enum values. Exporter protocol and
// endpoint values are validated only when the collector starts.
//
//	level: detailed -> level: bogus        caught
//	protocol: grpc  -> protocol: nonsense  NOT caught
func validateServiceTelemetry(t *testing.T, o ConfigOptions) error {
	t.Helper()

	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs: URIs(o),
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

	factories, err := Components()
	require.NoError(t, err)

	telCfg := factories.Telemetry.CreateDefaultConfig()
	if err := sub.Unmarshal(telCfg); err != nil {
		return err
	}
	return confmap.Validate(telCfg)
}

func TestCollectorURIsResolve(t *testing.T) {
	t.Run("local feature flag paths round trip", func(t *testing.T) {
		for _, file := range []string{"", "local flags.json", `C:\viewer\flags.json`, `flags #1 "local".json`} {
			o := testOptions()
			o.FeatureFlagsFile = file
			cfg, err := resolveConfig(t, o)
			require.NoError(t, err)
			ext := cfg.Extensions[component.NewID(duckdbextension.Type)].(*duckdbextension.Config)
			require.Equal(t, file, ext.FeatureFlagsFile)
		}
	})
	t.Run("imports advertise the configured HTTP receiver port", func(t *testing.T) {
		o := testOptions()
		o.HTTPPort = 54318
		cfg, err := resolveConfig(t, o)
		require.NoError(t, err)
		ext := cfg.Extensions[component.NewID(duckdbextension.Type)].(*duckdbextension.Config)
		require.Equal(t, o.HTTPPort, ext.OTLPHTTPPort)
	})
	t.Run("telemetry off", func(t *testing.T) {
		cfg, err := resolveConfig(t, testOptions())
		require.NoError(t, err)
		require.NoError(t, cfg.Validate())
	})

	t.Run("telemetry external", func(t *testing.T) {
		o := testOptions()
		o.SelfTelemetryEndpoint = "http://localhost:4327"

		cfg, err := resolveConfig(t, o)
		require.NoError(t, err)
		require.NoError(t, cfg.Validate())
	})
}

// The exporter and service telemetry settings must enable or disable together.
func TestServiceTelemetryValidates(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		require.NoError(t, validateServiceTelemetry(t, testOptions()))
	})

	t.Run("external", func(t *testing.T) {
		o := testOptions()
		o.SelfTelemetryEndpoint = "http://localhost:4327"
		require.NoError(t, validateServiceTelemetry(t, o))
	})
}

// The exporter must enable its own instrumentation as well as the providers.
func TestExternalTelemetrySetsExporterMode(t *testing.T) {
	off := telemetryURIs(testOptions())
	assert.NotContains(t, strings.Join(off, "\n"), "exporters::desktop::telemetry",
		"exporter telemetry should be left at its default when the flag is off")

	o := testOptions()
	o.SelfTelemetryEndpoint = "http://localhost:4327"
	on := strings.Join(telemetryURIs(o), "\n")

	assert.Contains(t, on, "exporters::desktop::telemetry: enabled")
	assert.Contains(t, on, "extensions::duckdb::telemetry: enabled")
}

// Off must keep metrics at level none. Without it the collector's default
// config stands up a Pull (Prometheus) reader, so the default build would open
// a metrics endpoint nobody asked for.
func TestTelemetryOffKeepsMetricsNone(t *testing.T) {
	uris := telemetryURIs(testOptions())
	require.Len(t, uris, 1)
	assert.Equal(t, `yaml:service::telemetry::metrics::level: none`, uris[0])
}

// The batch processor must be present because the sending queue does not batch.
func TestPipelinesBatch(t *testing.T) {
	joined := strings.Join(URIs(testOptions()), "\n")

	for _, signal := range []string{"traces", "metrics", "logs"} {
		assert.Contains(t, joined,
			"service::pipelines::"+signal+"::processors: [redaction, batch]",
			"%s pipeline must redact before batching", signal)
	}

	// send_batch_max_size bounds a merged batch, which is what makes the
	// exporter's IngestTimeout a meaningful deadline rather than a guess.
	assert.Contains(t, joined, "processors::batch::send_batch_max_size:")

	cfg, err := resolveConfig(t, testOptions())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
}

// The external OTLP target is independent of this viewer's own gRPC receiver.
func TestSelfTelemetryEndpointConfiguresBothSignalsExactly(t *testing.T) {
	o := testOptions()
	o.SelfTelemetryEndpoint = "http://collector.example:4327"
	o.GRPCPort = 15317
	o.Host = "127.0.0.1"

	joined := strings.Join(URIs(o), "\n")
	assert.Contains(t, joined, `receivers::otlp::protocols::grpc::endpoint: "127.0.0.1:15317"`)
	assert.Contains(t, joined, `extensions::duckdb::otlp_grpc_port: 15317`)
	assert.Equal(t, 2, strings.Count(joined, `endpoint: "http://collector.example:4327"`),
		"traces and metrics must use the exact supplied endpoint")
}

// TestComponentModuleVersionsMatchGoMod checks reported component versions
// against the dependency versions the binary uses.
func TestComponentModuleVersionsMatchGoMod(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	require.NoError(t, err)

	// module path -> version, from the require blocks.
	pinned := map[string]string{}
	for _, line := range strings.Split(string(gomod), "\n") {
		fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), " // indirect"))
		if len(fields) == 2 && strings.HasPrefix(fields[1], "v") {
			pinned[fields[0]] = fields[1]
		}
	}
	require.NotEmpty(t, pinned, "parsed no module versions out of go.mod")

	factories, err := Components()
	require.NoError(t, err)

	// Every collector module the binary advertises must name the pinned version.
	reported := map[component.Type]string{}
	for k, v := range factories.ReceiverModules {
		reported[k] = v
	}
	for k, v := range factories.ProcessorModules {
		reported[k] = v
	}
	for k, v := range factories.ExporterModules {
		reported[k] = v
	}
	for k, v := range factories.ExtensionModules {
		reported[k] = v
	}
	require.NotEmpty(t, reported)

	checked := 0
	for typ, decl := range reported {
		path, version, found := strings.Cut(decl, " ")
		if !found {
			// Our own modules are declared without a version, which is correct:
			// they are this repo, not a dependency.
			continue
		}
		want, ok := pinned[path]
		if !ok {
			continue
		}
		assert.Equalf(t, want, version,
			"component %q reports %s at %s, but go.mod pins %s", typ, path, version, want)
		checked++
	}
	assert.Positive(t, checked, "no versioned collector modules were compared")
}
