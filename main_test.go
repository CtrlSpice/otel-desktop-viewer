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
	return confmap.Validate(telCfg)
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

// Batching must actually be in the pipeline. The exporter's sending queue no
// longer batches, so if the processor is missing from the pipelines nothing
// batches at all -- every client request becomes its own appender transaction.
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

// TestStartupFailureIsNotAnsweredWithUsage covers the difference between "you
// typed the command wrong" and "the collector could not start".
//
// Cobra cannot tell them apart on its own: by default any error out of RunE
// gets the full flag listing and a second printing of the error. The common
// way this binary exits non-zero is someone upgrading across a schema change,
// and the sentence telling them what to do was landing under a screen of
// flags. Flags are parsed before RunE, so silencing there keeps usage for the
// case usage is actually for.
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
		assert.Contains(t, out.String(), "Usage:",
			"a mistyped flag is exactly what usage exists to explain")
	})

	// Asserting that RunE *arms* the silencing, not merely that the command
	// does not carry it. An earlier version of this test only checked the
	// latter, which is the default -- it passed with the fix reverted, and said
	// nothing at all.
	t.Run("reaching RunE arms the silencing", func(t *testing.T) {
		cmd := newCommand(set)
		require.False(t, cmd.SilenceUsage,
			"not on the command, or a mistyped flag would be silenced too")
		require.False(t, cmd.SilenceErrors)

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
		assert.NotContains(t, out.String(), "Usage:",
			"a startup failure must not be answered with the flag listing")
	})
}

// TestComponentModuleVersionsMatchGoMod keeps the versions this binary reports
// for its components in step with the ones it is actually built against.
//
// components.go carries them as literal strings and is labelled generated, but
// there is no manifest to regenerate it from -- it is hand-maintained, and a
// dependency bump touches go.mod without touching it. That is how the receiver
// and processor came to report v0.157.0 in a binary built against v0.158.0:
// `otel-desktop-viewer components` named a version that was not there, and
// nothing noticed for a whole release cycle.
//
// Reading go.mod rather than hardcoding the expected version, so this asserts
// agreement rather than becoming a third place to update.
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
