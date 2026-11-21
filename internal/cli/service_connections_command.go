package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// Service connections are directed parent-to-child edges from the viewer's service map.
func newServiceConnectionsCommand(client *http.Client, now func() time.Time) *cobra.Command {
	var options telemetrySearchOptions
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "connections <service-identity>", Short: "Show incoming and outgoing service connections",
		Long: "Use serviceIdentity from services to list recorded cross-service parent and child spans grouped by source and target. Both spans must be in the selected window.",
		Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now())
			if err != nil {
				return err
			}
			if q.Limit > 500 {
				return fmt.Errorf("--limit must be at most 500 for service map connections")
			}
			raw, err := requestViewerRPC(cmd.Context(), client, options.Endpoint, "discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connections", "serviceIdentity": args[0], "startTime": q.StartTime, "endTime": q.EndTime, "limit": q.Limit}})
			if err != nil {
				return err
			}
			var page struct {
				Rows      [][]json.RawMessage `json:"rows"`
				Truncated bool                `json:"truncated"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				return fmt.Errorf("decode service connections: %w", err)
			}
			entries := make([]json.RawMessage, 0, len(page.Rows))
			var rows [][]any
			for _, row := range page.Rows {
				if len(row) != 1 || isJSONNull(row[0]) {
					return fmt.Errorf("decode service connections: expected one connection per row")
				}
				entries = append(entries, row[0])
				if jsonOutput {
					continue
				}
				var edge struct {
					Source, Target                        serviceSummary
					SpanCount, TraceCount, ErrorSpanCount uint64
				}
				if err := json.Unmarshal(row[0], &edge); err != nil {
					return fmt.Errorf("decode service connection: %w", err)
				}
				rows = append(rows, []any{edge.Source.ServiceNamespace, edge.Source.ServiceName, edge.Source.ServiceIdentity, edge.Target.ServiceNamespace, edge.Target.ServiceName, edge.Target.ServiceIdentity, edge.SpanCount, edge.TraceCount, edge.ErrorSpanCount})
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"startTime": q.StartTime, "endTime": q.EndTime, "connections": entries, "truncated": page.Truncated})
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), formatQueryColumns(queryResult{Columns: []queryColumn{{Name: "sourceNamespace"}, {Name: "sourceName"}, {Name: "sourceIdentity"}, {Name: "targetNamespace"}, {Name: "targetName"}, {Name: "targetIdentity"}, {Name: "spanCount"}, {Name: "traceCount"}, {Name: "errorSpanCount"}}, Rows: rows, Truncated: page.Truncated}))
			return err
		},
	}
	addTelemetryWindowFlags(cmd, &options, &jsonOutput)
	cmd.Flags().Lookup("limit").Usage = "Maximum connections to return (1–500)"
	return cmd
}
