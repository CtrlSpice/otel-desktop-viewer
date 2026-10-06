package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
)

type runCollectorFunc func(context.Context, otelcol.CollectorSettings) error
type openURLFunc func(string) error

func runInteractive(set otelcol.CollectorSettings) error {
	return newCommand(set).Execute()
}

func newCommand(set otelcol.CollectorSettings) *cobra.Command {
	return newRootCommand(set, http.DefaultClient, time.Now, runCollector, browser.OpenURL)
}

func runCollector(ctx context.Context, set otelcol.CollectorSettings) error {
	collector, err := otelcol.NewCollector(set)
	if err != nil {
		return fmt.Errorf("create viewer: %w", err)
	}
	if err := collector.Run(ctx); err != nil {
		return fmt.Errorf("run viewer: %w", err)
	}
	return nil
}

func newRootCommand(
	set otelcol.CollectorSettings,
	client *http.Client,
	now func() time.Time,
	runViewer runCollectorFunc,
	openURL openURLFunc,
) *cobra.Command {
	options := configOptions{host: "localhost", httpPort: 4318, grpcPort: 4317, browserPort: 8000}
	openBrowser := true

	root := &cobra.Command{
		Use:           set.BuildInfo.Command,
		Short:         "View OpenTelemetry data locally or inspect a running viewer.",
		Version:       set.BuildInfo.Version,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings := prepareCollectorSettings(set, options)
			if openBrowser {
				go openViewerAfterDelay(cmd.Context(), options.host, options.browserPort, openURL)
			}
			return runViewer(cmd.Context(), settings)
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpFunc(writeRootHelp)
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		writeRootHelpTo(cmd, cmd.ErrOrStderr())
		return nil
	})
	root.PersistentFlags().BoolP("help", "h", false, "Help for this command")
	root.Flags().SortFlags = false
	root.Flags().StringVar(&options.host, "host", options.host, "Address used by viewer/OTLP receivers")
	root.Flags().IntVar(&options.httpPort, "http", options.httpPort, "OTLP HTTP port")
	root.Flags().IntVar(&options.grpcPort, "grpc", options.grpcPort, "OTLP gRPC port")
	root.Flags().IntVar(&options.browserPort, "browser-port", options.browserPort, "Viewer HTTP port")
	root.Flags().BoolVar(&openBrowser, "open-browser", openBrowser, "Open viewer after startup")
	root.Flags().StringVar(&options.db, "db", "", "DuckDB file; omit for memory")
	root.Flags().StringVar(&options.dbMaxSize, "db-max-size", "", "Maximum telemetry-store size (defaults to 512MB in memory or 2GB with --db; 0 disables pruning)")
	root.Flags().BoolVar(&options.selfTelemetry, "telemetry", false, "Send viewer telemetry to itself")

	commands := []*cobra.Command{
		newQueryCommand(client),
		newTracesCommand(client, now),
		newLogsCommand(client, now),
		newMetricsCommand(client, now),
	}
	for _, command := range commands {
		command.SetHelpFunc(writeCommandHelp)
		command.SetUsageFunc(func(cmd *cobra.Command) error {
			writeCommandHelpTo(cmd, cmd.ErrOrStderr())
			return nil
		})
		root.AddCommand(command)
	}
	root.InitDefaultHelpCmd()
	for _, command := range root.Commands() {
		if command.Name() == "help" {
			command.Hidden = true
		}
	}
	return root
}

func openViewerAfterDelay(ctx context.Context, host string, port int, openURL openURLFunc) {
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		_ = openURL("http://" + browserHostFor(host) + ":" + strconv.Itoa(port) + "/")
	}
}

func writeRootHelp(cmd *cobra.Command, _ []string) {
	writeRootHelpTo(cmd, cmd.OutOrStdout())
}

func writeRootHelpTo(cmd *cobra.Command, writer io.Writer) {
	_, _ = fmt.Fprintf(writer, `%s

View OpenTelemetry data locally or inspect a running viewer.

The bare command runs the viewer in the foreground until it receives a signal.
Client commands require a running viewer.

For automation, reuse an existing viewer. Otherwise, start
  %s --open-browser=false
as a managed child. Retain its process handle, wait for the configured viewer
HTTP endpoint to accept connections, then terminate and wait for the child only
if you started it. Never stop a viewer you did not start.

USAGE
  %s [flags]
  %s <command> [flags]

COMMANDS
  query     🔎 Run SQL against a running viewer
  traces    🧵 Search traces in a running viewer
  logs      🪵 Search logs in a running viewer
  metrics   📈 Search metrics in a running viewer

VIEWER FLAGS
      --host string          Address used by viewer/OTLP receivers (default "localhost")
      --http int             OTLP HTTP port (default 4318)
      --grpc int             OTLP gRPC port (default 4317)
      --browser-port int     Viewer HTTP port (default 8000)
      --open-browser         Open viewer after startup (default true)
      --db string            DuckDB file; omit for memory
      --db-max-size string   Maximum telemetry-store size (defaults to 512MB in memory or 2GB with --db; 0 disables pruning)
      --telemetry            Send viewer telemetry to itself

GLOBAL FLAGS
  -h, --help      Help for this command
  -v, --version   Version for %s
`, cmd.CommandPath(), cmd.CommandPath(), cmd.CommandPath(), cmd.CommandPath(), cmd.CommandPath())
}

func writeCommandHelp(cmd *cobra.Command, _ []string) {
	writeCommandHelpTo(cmd, cmd.OutOrStdout())
}

func writeCommandHelpTo(cmd *cobra.Command, writer io.Writer) {
	_, _ = fmt.Fprintln(writer, cmd.Long)
	_, _ = fmt.Fprintf(writer, "\nUSAGE\n  %s\n", cmd.UseLine())
	if cmd.Example != "" {
		_, _ = fmt.Fprintf(writer, "\nEXAMPLES\n%s\n", cmd.Example)
	}
	if cmd.HasAvailableLocalFlags() {
		_, _ = fmt.Fprintf(writer, "\nFLAGS\n%s", cmd.LocalNonPersistentFlags().FlagUsages())
	}
	if cmd.HasAvailableInheritedFlags() {
		_, _ = fmt.Fprintf(writer, "\nGLOBAL FLAGS\n%s", cmd.InheritedFlags().FlagUsages())
	}
}
