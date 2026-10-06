package main

import (
	"time"

	"github.com/spf13/cobra"
)

var traceSummaryFields = []string{
	"traceID",
	"hasRootSpan",
	"rootSpan",
	"startTime",
	"durationNs",
	"spanCount",
	"errorCount",
}

func newTracesCommand(now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "🔭 Search trace summaries in the running viewer",
		Long: "🔭 Search existing trace summaries in the running viewer. " +
			"The default window is the last hour; output uses aligned columns unless --json is set.",
		Example: "  otel-desktop-viewer traces\n" +
			"  otel-desktop-viewer traces --service checkout --since 30m\n" +
			"  otel-desktop-viewer traces --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			query, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now())
			if err != nil {
				return err
			}
			result, err := requestTelemetrySearch(cmd.Context(), queryHTTPClient, options.Endpoint, "searchTraces", query, traceSummaryFields)
			if err != nil {
				return err
			}
			return writeTelemetrySearchResult(cmd.OutOrStdout(), result, traceSummaryFields, jsonOutput)
		},
	}
	addTelemetrySearchFlags(cmd, &options, &jsonOutput)
	return cmd
}
