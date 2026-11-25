package cli

import (
	"maps"
	"net/http"
	"slices"
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

var filteredTraceSummaryFields = append(
	append([]string(nil), traceSummaryFields...),
	"matchedSpans",
)

func newTracesCommand(client *http.Client, now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "🧵 Search traces in a running viewer",
		Long: "🧵 Search existing trace summaries in the running viewer. " +
			"The default window is the last hour; output uses aligned columns unless --json is set. " +
			"With --service, matches use the row’s trace ID unless shown. --json includes both IDs.",
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
			fields := traceSummaryFields
			if options.Service != "" {
				fields = filteredTraceSummaryFields
			}
			result, err := requestTelemetrySearch(cmd.Context(), client, options.Endpoint, "searchTraceSummaries", query, fields)
			if err != nil {
				return err
			}
			if options.Service != "" && !jsonOutput {
				matchColumn := slices.Index(fields, "matchedSpans")
				traceColumn := slices.Index(fields, "traceID")
				for _, row := range result.Rows {
					row[matchColumn] = matchedSpansForTable(row[traceColumn], row[matchColumn])
				}
				fields = slices.Clone(fields)
				fields[matchColumn] = "matchedSpans (row traceID omitted)"
			}
			return writeProjectedSearchResult(cmd.OutOrStdout(), result, fields, jsonOutput)
		},
	}
	addTelemetrySearchFlags(cmd, &options, &jsonOutput)
	return cmd
}

// Only an exact match makes a nested trace ID redundant. Keep unfamiliar
// fields and any explicit reference that cannot be recovered from the row.
func matchedSpansForTable(owner, value any) any {
	traceID, ok := owner.(string)
	matches, list := value.([]any)
	if !ok || traceID == "" || !list {
		return value
	}
	display := slices.Clone(matches)
	for i, item := range matches {
		match, ok := item.(map[string]any)
		if !ok || match["traceID"] != traceID {
			continue
		}
		entry := maps.Clone(match)
		delete(entry, "traceID")
		display[i] = entry
	}
	return display
}
