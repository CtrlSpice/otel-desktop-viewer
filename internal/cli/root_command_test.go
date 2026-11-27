package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/otelcol"
)

func TestCommandHelpSnapshotsGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "root", args: []string{"--help"}},
		{name: "query", args: []string{"query", "--help"}},
		{name: "traces", args: []string{"traces", "--help"}},
		{name: "logs", args: []string{"logs", "--help"}},
		{name: "metrics", args: []string{"metrics", "--help"}},
		{name: "skills", args: []string{"skills", "--help"}},
		{name: "services", args: []string{"services", "--help"}},
		{name: "attributes", args: []string{"attributes", "--help"}},
		{name: "attributes_keys", args: []string{"attributes", "keys", "--help"}},
		{name: "attributes_values", args: []string{"attributes", "values", "--help"}},
		{name: "export_trace", args: []string{"export", "trace", "--help"}},
		{name: "import", args: []string{"import", "--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := false
			out, _, err := execCLI(t, cliOptions{
				client:   offlineHTTPClient(t),
				mergeErr: true,
				runViewer: func(context.Context, otelcol.CollectorSettings) error {
					started = true
					return nil
				},
			}, tc.args...)
			require.NoError(t, err)
			assertGolden(t, "help_"+tc.name, out)
			assert.False(t, started)
		})
	}
}

func TestSkillsCommandPrintsBundledGuide(t *testing.T) {
	expected, err := os.ReadFile("../../skills/otel-desktop-viewer/SKILL.md")
	require.NoError(t, err)

	out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t), mergeErr: true}, "skills")
	require.NoError(t, err)
	assert.Equal(t, string(expected), out)
}

func TestHiddenHelpCommandRemainsCallable(t *testing.T) {
	out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t), mergeErr: true}, "help", "query")
	require.NoError(t, err)
	assertGolden(t, "help_query", out)
}

func TestUsageErrorsAndRuntimeErrorsHaveDistinctOutput(t *testing.T) {
	noopViewer := func(context.Context, otelcol.CollectorSettings) error { return nil }
	noopOpen := func(string) error { return nil }

	for _, args := range [][]string{{"unknown"}, {"--not-a-flag"}, {"query"}, {"traces", "unexpected"}} {
		out, _, err := execCLI(t, cliOptions{
			client:    offlineHTTPClient(t),
			runViewer: noopViewer,
			openURL:   noopOpen,
			mergeErr:  true,
		}, args...)
		require.Error(t, err)
		assert.Contains(t, out, "USAGE")
	}

	runtimeFailure := errors.New("database unavailable")
	out, _, err := execCLI(t, cliOptions{
		client:    offlineHTTPClient(t),
		runViewer: func(context.Context, otelcol.CollectorSettings) error { return runtimeFailure },
		openURL:   noopOpen,
		mergeErr:  true,
	}, "--open-browser=false")
	require.ErrorIs(t, err, runtimeFailure)
	assert.Empty(t, out, "the process boundary owns the single error print")
	assert.NotContains(t, err.Error(), "collector server run finished")
}

// TestStartupFailureIsNotAnsweredWithUsage distinguishes command errors from
// collector startup failures. Usage remains enabled until flag parsing succeeds.
func TestStartupFailureIsNotAnsweredWithUsage(t *testing.T) {
	set := collector.Settings("test")

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

		// Enter RunE with a config that cannot start.
		cmd.SetArgs([]string{"--open-browser=false", "--db", filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")})
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

func TestViewerStartupUsesParsedFlagsAndBrowserContext(t *testing.T) {
	var defaults otelcol.CollectorSettings
	_, _, err := execCLI(t, cliOptions{
		runViewer: func(_ context.Context, set otelcol.CollectorSettings) error {
			defaults = set
			return nil
		},
	}, "--open-browser=false")
	require.NoError(t, err)
	assert.Equal(t, collector.URIs(collector.ConfigOptions{Host: "localhost", HTTPPort: 4318, GRPCPort: 4317, BrowserPort: 8000}), defaults.ConfigProviderSettings.ResolverSettings.URIs)

	for _, badDB := range []string{"", "   ", ":memory:", ":MEMORY:"} {
		t.Run("rejects invalid --db "+badDB, func(t *testing.T) {
			_, _, err := execCLI(t, cliOptions{
				runViewer: func(context.Context, otelcol.CollectorSettings) error {
					t.Fatalf("--db %q should have failed before starting the viewer", badDB)
					return nil
				},
				openURL: func(string) error {
					t.Fatalf("--db %q opened a browser", badDB)
					return nil
				},
			}, "--open-browser=false", "--db", badDB)
			require.Error(t, err)
		})
	}

	var received otelcol.CollectorSettings
	opened := make(chan string, 1)
	_, _, err = execCLI(t, cliOptions{
		runViewer: func(_ context.Context, set otelcol.CollectorSettings) error {
			received = set
			return nil
		},
		openURL: func(url string) error {
			opened <- url
			return nil
		},
	}, "--host", "0.0.0.0", "--http", "14318", "--grpc", "14317", "--browser-port", "18000", "--db", "viewer.duckdb", "--db-max-size", "3GB", "--self-telemetry-endpoint", "http://localhost:4327")
	require.NoError(t, err)
	expected := collector.ConfigOptions{Host: "0.0.0.0", HTTPPort: 14318, GRPCPort: 14317, BrowserPort: 18000, DB: "viewer.duckdb", DBMaxSize: "3GB", SelfTelemetryEndpoint: "http://localhost:4327"}
	assert.Equal(t, collector.URIs(expected), received.ConfigProviderSettings.ResolverSettings.URIs)
	assert.Equal(t, "env", received.ConfigProviderSettings.ResolverSettings.DefaultScheme)
	select {
	case url := <-opened:
		assert.Equal(t, "http://localhost:18000/", url)
	case <-time.After(time.Second):
		t.Fatal("viewer browser URL was not opened")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	openViewerAfterDelay(ctx, "localhost", 8000, func(string) error {
		t.Fatal("cancelled browser launch opened a URL")
		return nil
	})
}

func TestRemovedTelemetryFlagIsRejected(t *testing.T) {
	_, _, err := execCLI(t, cliOptions{openURL: func(string) error { return nil }}, "--telemetry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --telemetry")
}

func TestRootExposesOnlyViewerCommandsAndFlags(t *testing.T) {
	cmd := newCommand(testCollectorSettings())
	var names []string
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			names = append(names, child.Name())
		}
	}
	assert.Equal(t, []string{"attributes", "export", "import", "log", "logs", "metric", "metrics", "query", "services", "skills", "span", "trace", "traces"}, names)
	for _, forbidden := range []string{"config", "set", "feature-gates"} {
		assert.Nil(t, cmd.Flags().Lookup(forbidden))
	}
	for _, forbidden := range []string{"components", "featuregate", "validate", "print-config", "completion", "run", "serve"} {
		_, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, forbidden)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown command")
	}
}

func TestViewerReceivesCommandCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := execCLI(t, cliOptions{
		ctx: ctx,
		runViewer: func(runContext context.Context, _ otelcol.CollectorSettings) error {
			return runContext.Err()
		},
		openURL: func(string) error { return nil },
	}, "--open-browser=false")
	require.ErrorIs(t, err, context.Canceled)
}
