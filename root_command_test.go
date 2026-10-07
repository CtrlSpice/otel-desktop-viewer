package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

const expectedRootHelp = `otel-desktop-viewer

View OpenTelemetry data locally or inspect a running viewer.

The bare command runs the viewer in the foreground until it receives a signal.
Client commands require a running viewer.

For automation, reuse an existing viewer. Otherwise, start
  otel-desktop-viewer --open-browser=false
as a managed child. Retain its process handle, wait for the configured viewer
HTTP endpoint to accept connections, then terminate and wait for the child only
if you started it. Never stop a viewer you did not start.

To observe the viewer itself, keep a monitoring viewer running:
  otel-desktop-viewer --grpc 4327 --http 4328 --browser-port 8001
Then start the observed viewer in another terminal:
  otel-desktop-viewer --self-telemetry-endpoint http://localhost:4327
Omitting --self-telemetry-endpoint keeps self-telemetry off. The monitoring
endpoint must remain running through observed viewer shutdown. The caller owns
starting, stopping, and waiting for both foreground processes.

USAGE
  otel-desktop-viewer [flags]
  otel-desktop-viewer <command> [flags]

COMMANDS
  attributes 🔎 Discover span attribute keys and values
  query     🔎 Run SQL against a running viewer
  traces    🧵 Search traces in a running viewer
  trace     🧵 Inspect one complete trace
  span      🧵 Inspect one span
  logs      🪵 Search logs in a running viewer
  metrics   📈 Search metrics in a running viewer
  skills    🧩 Print the agent usage guide

VIEWER FLAGS
      --host string          Address used by viewer/OTLP receivers (default "localhost")
      --http int             OTLP HTTP port (default 4318)
      --grpc int             OTLP gRPC port (default 4317)
      --browser-port int     Viewer HTTP port (default 8000)
      --open-browser         Open viewer after startup (default true)
      --db string            DuckDB file; omit for memory
      --db-max-size string   Maximum telemetry-store size (defaults to 512MB in memory or 2GB with --db; 0 disables pruning)
      --self-telemetry-endpoint string
                            Export the viewer's own traces and metrics to this OTLP/gRPC endpoint

GLOBAL FLAGS
  -h, --help      Help for this command
  -v, --version   Version for otel-desktop-viewer
`

const expectedQueryHelp = `🔎 Run one read-only DuckDB query against the existing viewer process. Results use aligned columns by default; --json emits the JSON result.

USAGE
  otel-desktop-viewer query <sql> [flags]

EXAMPLES
  otel-desktop-viewer query 'SHOW TABLES'
  otel-desktop-viewer query 'DESCRIBE spans'
  otel-desktop-viewer query 'SELECT service_name, count(*) FROM spans GROUP BY service_name' --limit 50
  otel-desktop-viewer query 'SELECT count(*) FROM logs' --json

FLAGS
      --endpoint string   Running viewer HTTP endpoint (default "http://localhost:8000")
      --json              Emit the JSON result instead of columns
      --limit uint        Maximum rows to return (default 25)

GLOBAL FLAGS
  -h, --help   Help for this command
`

const expectedSkillsHelp = `🧩 Print the bundled OTel Desktop Viewer agent usage guide.

USAGE
  otel-desktop-viewer skills [flags]

GLOBAL FLAGS
  -h, --help   Help for this command
`

func expectedTelemetryHelp(command, emoji, signal string, detail ...string) string {
	description := emoji + " Search existing " + signal + " summaries in the running viewer. The default window is the last hour; output uses aligned columns unless --json is set."
	if len(detail) > 0 {
		description += " " + detail[0]
	}
	return description + "\n\n" +
		"USAGE\n  otel-desktop-viewer " + command + " [flags]\n\n" +
		"EXAMPLES\n" +
		"  otel-desktop-viewer " + command + "\n" +
		"  otel-desktop-viewer " + command + " --service checkout --since 30m\n" +
		"  otel-desktop-viewer " + command + " --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z --json\n\n" +
		"FLAGS\n" +
		"      --end string        Inclusive RFC3339 end time (nanoseconds supported)\n" +
		"      --endpoint string   Running viewer HTTP endpoint (default \"http://localhost:8000\")\n" +
		"      --json              Emit the summary response as JSON instead of columns\n" +
		"      --limit int         Maximum summaries to return (default 25)\n" +
		"      --service string    Only summaries for this service\n" +
		"      --since duration    Relative lookback window (for example 30m, 6h, or 24h) (default 1h0m0s)\n" +
		"      --start string      Inclusive RFC3339 start time (nanoseconds supported)\n\n" +
		"GLOBAL FLAGS\n  -h, --help   Help for this command\n"
}

func TestCommandHelpSnapshotsAreOffline(t *testing.T) {
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	for _, test := range []struct {
		name     string
		args     []string
		expected string
	}{
		{name: "root", args: []string{"--help"}, expected: expectedRootHelp},
		{name: "query", args: []string{"query", "--help"}, expected: expectedQueryHelp},
		{name: "traces", args: []string{"traces", "--help"}, expected: expectedTelemetryHelp("traces", "🧵", "trace")},
		{name: "logs", args: []string{"logs", "--help"}, expected: expectedTelemetryHelp("logs", "🪵", "log")},
		{name: "metrics", args: []string{"metrics", "--help"}, expected: expectedTelemetryHelp("metrics", "📈", "metric", "JSON identifies a viewer-assigned Metric with the opaque metricRef field.")},
		{name: "skills", args: []string{"skills", "--help"}, expected: expectedSkillsHelp},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := false
			cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(context.Context, otelcol.CollectorSettings) error {
				started = true
				return nil
			}, func(string) error {
				t.Fatal("help opened a browser")
				return nil
			})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(test.args)
			require.NoError(t, cmd.Execute())
			assert.Equal(t, test.expected, output.String())
			assert.False(t, started)
		})
	}
}

func TestSkillsCommandPrintsBundledGuide(t *testing.T) {
	expected, err := os.ReadFile("skills/otel-desktop-viewer/SKILL.md")
	require.NoError(t, err)

	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(context.Context, otelcol.CollectorSettings) error {
		t.Fatal("skills started the viewer")
		return nil
	}, func(string) error {
		t.Fatal("skills opened a browser")
		return nil
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"skills"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, expected, output.Bytes())
}

func TestHiddenHelpCommandRemainsCallable(t *testing.T) {
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(context.Context, otelcol.CollectorSettings) error {
		t.Fatal("help started the viewer")
		return nil
	}, func(string) error { return nil })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"help", "query"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, expectedQueryHelp, output.String())
}

func TestUsageErrorsAndRuntimeErrorsHaveDistinctOutput(t *testing.T) {
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	newTestCommand := func(runViewer runCollectorFunc) (*bytes.Buffer, *cobra.Command) {
		output := &bytes.Buffer{}
		cmd := newRootCommand(settings, http.DefaultClient, time.Now, runViewer, func(string) error { return nil })
		cmd.SetOut(output)
		cmd.SetErr(output)
		return output, cmd
	}

	for _, args := range [][]string{{"unknown"}, {"--not-a-flag"}, {"query"}, {"traces", "unexpected"}} {
		output, cmd := newTestCommand(func(context.Context, otelcol.CollectorSettings) error { return nil })
		cmd.SetArgs(args)
		require.Error(t, cmd.Execute())
		assert.Contains(t, output.String(), "USAGE")
	}

	runtimeFailure := errors.New("database unavailable")
	output, cmd := newTestCommand(func(context.Context, otelcol.CollectorSettings) error { return runtimeFailure })
	cmd.SetArgs([]string{"--open-browser=false"})
	err := cmd.Execute()
	require.ErrorIs(t, err, runtimeFailure)
	assert.Empty(t, output.String(), "the process boundary owns the single error print")
	assert.NotContains(t, err.Error(), "collector server run finished")
}

func TestViewerStartupUsesParsedFlagsAndBrowserContext(t *testing.T) {
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	var defaults otelcol.CollectorSettings
	defaultCommand := newRootCommand(settings, http.DefaultClient, time.Now, func(_ context.Context, set otelcol.CollectorSettings) error {
		defaults = set
		return nil
	}, func(string) error {
		t.Fatal("--open-browser=false opened a URL")
		return nil
	})
	defaultCommand.SetArgs([]string{"--open-browser=false"})
	require.NoError(t, defaultCommand.Execute())
	assert.Equal(t, collectorURIs(testOptions()), defaults.ConfigProviderSettings.ResolverSettings.URIs)

	var received otelcol.CollectorSettings
	opened := make(chan string, 1)
	cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(_ context.Context, set otelcol.CollectorSettings) error {
		received = set
		return nil
	}, func(url string) error {
		opened <- url
		return nil
	})
	cmd.SetArgs([]string{"--host", "0.0.0.0", "--http", "14318", "--grpc", "14317", "--browser-port", "18000", "--db", "viewer.duckdb", "--db-max-size", "3GB", "--self-telemetry-endpoint", "http://localhost:4327"})
	require.NoError(t, cmd.Execute())
	expected := configOptions{host: "0.0.0.0", httpPort: 14318, grpcPort: 14317, browserPort: 18000, db: "viewer.duckdb", dbMaxSize: "3GB", selfTelemetryEndpoint: "http://localhost:4327"}
	assert.Equal(t, collectorURIs(expected), received.ConfigProviderSettings.ResolverSettings.URIs)
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
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(context.Context, otelcol.CollectorSettings) error {
		t.Fatal("removed flag started the viewer")
		return nil
	}, func(string) error { return nil })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--telemetry"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --telemetry")
}

func TestRootExposesOnlyViewerCommandsAndFlags(t *testing.T) {
	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}})
	var names []string
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			names = append(names, child.Name())
		}
	}
	assert.Equal(t, []string{"attributes", "logs", "metrics", "query", "skills", "span", "trace", "traces"}, names)
	for _, forbidden := range []string{"config", "set", "feature-gates"} {
		assert.Nil(t, cmd.Flags().Lookup(forbidden))
	}
	for _, forbidden := range []string{"components", "featuregate", "validate", "print-config", "completion", "run", "serve"} {
		candidate := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}})
		candidate.SetOut(&bytes.Buffer{})
		candidate.SetErr(&bytes.Buffer{})
		candidate.SetArgs([]string{forbidden})
		err := candidate.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown command")
	}
}

func TestViewerReceivesCommandCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settings := otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"}}
	cmd := newRootCommand(settings, http.DefaultClient, time.Now, func(runContext context.Context, _ otelcol.CollectorSettings) error {
		return runContext.Err()
	}, func(string) error { return nil })
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--open-browser=false"})
	require.ErrorIs(t, cmd.Execute(), context.Canceled)
}
