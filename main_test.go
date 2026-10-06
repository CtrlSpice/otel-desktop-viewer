package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	envprovider "go.opentelemetry.io/collector/confmap/provider/envprovider"
	yamlprovider "go.opentelemetry.io/collector/confmap/provider/yamlprovider"
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
// This validates the block's shape and enum values. Exporter protocol and
// endpoint values are validated only when the collector starts.
//
//	level: detailed -> level: bogus        caught
//	protocol: grpc  -> protocol: nonsense  NOT caught
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
	return confmap.Validate(telCfg)
}

func TestCollectorURIsResolve(t *testing.T) {
	t.Run("telemetry off", func(t *testing.T) {
		cfg, err := resolveConfig(t, testOptions())
		require.NoError(t, err)
		require.NoError(t, cfg.Validate())
	})

	t.Run("telemetry external", func(t *testing.T) {
		o := testOptions()
		o.selfTelemetryEndpoint = "http://localhost:4327"

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
		o.selfTelemetryEndpoint = "http://localhost:4327"
		require.NoError(t, validateServiceTelemetry(t, o))
	})
}

// The exporter must enable its own instrumentation as well as the providers.
func TestExternalTelemetrySetsExporterMode(t *testing.T) {
	off := telemetryURIs(testOptions())
	assert.NotContains(t, strings.Join(off, "\n"), "exporters::desktop::telemetry",
		"exporter telemetry should be left at its default when the flag is off")

	o := testOptions()
	o.selfTelemetryEndpoint = "http://localhost:4327"
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
	joined := strings.Join(collectorURIs(testOptions()), "\n")

	for _, signal := range []string{"traces", "metrics", "logs"} {
		assert.Contains(t, joined,
			"service::pipelines::"+signal+"::processors: [batch]",
			"%s pipeline must batch", signal)
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
	o.selfTelemetryEndpoint = "http://collector.example:4327"
	o.grpcPort = 15317
	o.host = "127.0.0.1"

	joined := strings.Join(collectorURIs(o), "\n")
	assert.Contains(t, joined, `receivers::otlp::protocols::grpc::endpoint: "127.0.0.1:15317"`)
	assert.Equal(t, 2, strings.Count(joined, `endpoint: "http://collector.example:4327"`),
		"traces and metrics must use the exact supplied endpoint")
}

// TestStartupFailureIsNotAnsweredWithUsage distinguishes command errors from
// collector startup failures. Usage remains enabled until flag parsing succeeds.
func TestStartupFailureIsNotAnsweredWithUsage(t *testing.T) {
	// The same settings main() builds. A bare struct has no config providers,
	// and NewCollector answers that with log.Fatal -- which takes the test
	// process with it before anything can be asserted.
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"},
		Factories: components,
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				ProviderFactories: []confmap.ProviderFactory{
					envprovider.NewFactory(),
					yamlprovider.NewFactory(),
				},
			},
		},
	}

	t.Run("a bad flag still prints usage", func(t *testing.T) {
		cmd := newCommand(set)
		cmd.SetArgs([]string{"--nonsense-flag"})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, out.String(), "USAGE",
			"a mistyped flag is exactly what usage exists to explain")
	})

	// RunE must enable usage silencing; its initial value alone proves nothing.
	t.Run("reaching RunE arms the silencing", func(t *testing.T) {
		cmd := newCommand(set)
		require.False(t, cmd.SilenceUsage,
			"not on the command, or a mistyped flag would be silenced too")
		require.True(t, cmd.SilenceErrors,
			"the process boundary prints returned errors once")

		// Run with flags that parse but a config that cannot start, so RunE is
		// entered and fails. What it did to the command on the way in is the
		// thing under test.
		cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		_ = cmd.Execute()

		assert.True(t, cmd.SilenceUsage,
			"RunE must silence usage: past flag parsing, a failure is not a usage mistake")
		assert.True(t, cmd.SilenceErrors)
		assert.NotContains(t, out.String(), "USAGE",
			"a startup failure must not be answered with the flag listing")
	})
}

// TestComponentModuleVersionsMatchGoMod checks reported component versions
// against the dependency versions the binary uses.
func TestComponentModuleVersionsMatchGoMod(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	require.NoError(t, err)

	// module path -> version, from the require blocks.
	pinned := map[string]string{}
	for _, line := range strings.Split(string(gomod), "\n") {
		fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), " // indirect"))
		if len(fields) == 2 && strings.HasPrefix(fields[0], "go.opentelemetry.io/") &&
			strings.HasPrefix(fields[1], "v") {
			pinned[fields[0]] = fields[1]
		}
	}
	require.NotEmpty(t, pinned, "parsed no module versions out of go.mod")

	factories, err := components()
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
