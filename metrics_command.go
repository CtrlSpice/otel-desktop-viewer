package main

import (
	"time"

	"github.com/spf13/cobra"
)

var metricSummaryFields = []string{
	"id",
	"name",
	"description",
	"unit",
	"metricType",
	"aggregationTemporalityCode",
	"aggregationTemporality",
	"isMonotonic",
	"serviceName",
	"seriesCount",
	"seriesCardinality",
	"dataPointCount",
	"lastValue",
	"lastSeen",
}

func newMetricsCommand(now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "📈 Search metric summaries in the running viewer",
		Long: "📈 Search existing metric summaries in the running viewer. " +
			"The default window is the last hour; output uses aligned columns unless --json is set.",
		Example: "  otel-desktop-viewer metrics\n" +
			"  otel-desktop-viewer metrics --service checkout --since 30m\n" +
			"  otel-desktop-viewer metrics --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			query, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now())
			if err != nil {
				return err
			}
			result, err := requestTelemetrySearch(cmd.Context(), queryHTTPClient, options.Endpoint, "searchMetricSummaries", query, metricSummaryFields)
			if err != nil {
				return err
			}
			return writeTelemetrySearchResult(cmd.OutOrStdout(), result, metricSummaryFields, jsonOutput)
		},
	}
	addTelemetrySearchFlags(cmd, &options, &jsonOutput)
	return cmd
}
