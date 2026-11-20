package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var metricSummaryFields = []string{
	"metricRef",
	"name",
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

func newMetricsCommand(client *http.Client, now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "📈 Search metrics in a running viewer",
		Long: "📈 Search existing metric summaries in the running viewer. " +
			"The default window is the last hour; output uses aligned columns unless --json is set. " +
			"Use metricRef to inspect or export a Metric.",
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
			result, err := requestTelemetrySearch(cmd.Context(), client, options.Endpoint, "searchMetricSummaries", query, metricSummaryFields)
			if err != nil {
				return err
			}
			return writeMetricSearchResult(cmd.OutOrStdout(), result, jsonOutput)
		},
	}
	addTelemetrySearchFlags(cmd, &options, &jsonOutput)
	return cmd
}

func writeMetricSearchResult(writer io.Writer, result telemetrySearchResult, jsonOutput bool) error {
	if !jsonOutput {
		return writeTelemetrySearchResult(writer, result, metricSummaryFields, false)
	}
	summaries, err := metricSummariesForCLI(result.Summaries)
	if err != nil {
		return fmt.Errorf("prepare metric JSON output: %w", err)
	}
	result.Summaries = summaries
	return writeTelemetrySearchResult(writer, result, metricSummaryFields, true)
}

func metricSummariesForCLI(summaries []json.RawMessage) ([]json.RawMessage, error) {
	projected := make([]json.RawMessage, len(summaries))
	for index, summary := range summaries {
		fields := make(map[string]json.RawMessage, len(metricSummaryFields))
		decoder := json.NewDecoder(bytes.NewReader(summary))
		if err := decoder.Decode(&fields); err != nil {
			return nil, fmt.Errorf("decode summary %d: %w", index, err)
		}

		var output bytes.Buffer
		output.WriteByte('{')
		for fieldIndex, field := range metricSummaryFields {
			value, ok := fields[field]
			if !ok {
				return nil, fmt.Errorf("summary %d is missing field %q", index, field)
			}
			if fieldIndex > 0 {
				output.WriteByte(',')
			}
			encodedField, err := json.Marshal(field)
			if err != nil {
				return nil, err
			}
			output.Write(encodedField)
			output.WriteByte(':')
			output.Write(value)
		}
		output.WriteByte('}')
		projected[index] = output.Bytes()
	}
	return projected, nil
}
