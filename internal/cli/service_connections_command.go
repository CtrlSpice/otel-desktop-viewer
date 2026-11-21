package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/spf13/cobra"
)

type connectionEndpoint struct {
	ServiceIdentity  string
	ServiceNamespace *string
	ServiceName      *string
}

func decodeConnectionPage(raw json.RawMessage) ([]json.RawMessage, [][]any, bool, error) {
	var page struct {
		Rows      [][]json.RawMessage
		Truncated *bool
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, nil, false, err
	}
	if page.Rows == nil || page.Truncated == nil {
		return nil, nil, false, fmt.Errorf("expected rows and truncated")
	}
	entries := make([]json.RawMessage, 0, len(page.Rows))
	var rows [][]any
	for i, row := range page.Rows {
		var edge struct {
			Source, Target                        connectionEndpoint
			SpanCount, TraceCount, ErrorSpanCount *uint64
		}
		if len(row) != 1 {
			return nil, nil, false, fmt.Errorf("row %d: expected one connection", i)
		}
		if err := json.Unmarshal(row[0], &edge); err != nil {
			return nil, nil, false, fmt.Errorf("row %d: %w", i, err)
		}
		s, t := edge.Source, edge.Target
		if s.ServiceIdentity == "" || t.ServiceIdentity == "" || s.ServiceName == nil || t.ServiceName == nil || s.ServiceNamespace == nil || t.ServiceNamespace == nil ||
			edge.SpanCount == nil || edge.TraceCount == nil || edge.ErrorSpanCount == nil {
			return nil, nil, false, fmt.Errorf("row %d: missing endpoint or count", i)
		}
		entries = append(entries, row[0])
		rows = append(rows, []any{*s.ServiceNamespace, *s.ServiceName, s.ServiceIdentity, *t.ServiceNamespace, *t.ServiceName, t.ServiceIdentity, *edge.SpanCount, *edge.TraceCount, *edge.ErrorSpanCount})
	}
	return entries, rows, *page.Truncated, nil
}

// Service connections are directed parent-to-child edges from the viewer's service map.
func newServiceConnectionsCommand(client *http.Client, now func() time.Time) *cobra.Command {
	var options telemetrySearchOptions
	var jsonOutput bool
	var offset uint64
	cmd := &cobra.Command{
		Use: "connections <service-identity>", Short: "Show incoming and outgoing service connections",
		Long: "Use serviceIdentity from services to list recorded cross-service parent and child spans grouped by source and target. Both spans must be in the selected window.",
		Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := discovery.NormalizeServiceRef(args[0])
			if err != nil {
				return err
			}
			at := now()
			q, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), at)
			if err != nil {
				return err
			}
			if q.Limit > 500 {
				return fmt.Errorf("--limit must be at most 500 for service map connections")
			}
			if offset > 1_000_000_000 {
				return fmt.Errorf("--offset must be at most 1000000000")
			}
			raw, err := requestViewerRPC(cmd.Context(), client, options.Endpoint, "discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connections", "serviceIdentity": ref, "startTime": q.StartTime, "endTime": q.EndTime, "limit": q.Limit, "offset": offset}})
			if err != nil {
				return err
			}
			entries, rows, truncated, err := decodeConnectionPage(raw)
			if err != nil {
				return fmt.Errorf("decode service connections: %w", err)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"startTime": q.StartTime, "endTime": q.EndTime, "connections": entries, "truncated": truncated})
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), formatQueryColumns(queryResult{Columns: []queryColumn{{Name: "sourceNamespace"}, {Name: "sourceName"}, {Name: "sourceIdentity"}, {Name: "targetNamespace"}, {Name: "targetName"}, {Name: "targetIdentity"}, {Name: "spanCount"}, {Name: "traceCount"}, {Name: "errorSpanCount"}}, Rows: rows}))
			if err == nil && truncated {
				next := offset + uint64(len(rows))
				if next <= 1_000_000_000 {
					start, end := options.Start, options.End
					if start == "" && end == "" {
						start, end = at.Add(-options.Since).UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano)
					}
					flags := fmt.Sprintf("--offset %d", next)
					if start != "" {
						flags += " --start " + start
					}
					if end != "" {
						flags += " --end " + end
					}
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "[next page: use %s instead of relative time]\n", flags)
				} else {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "[more connections available; narrow the time window to stay within the offset limit]")
				}
			}
			return err
		},
	}
	addTelemetryWindowFlags(cmd, &options, &jsonOutput)
	cmd.Flags().Uint64Var(&offset, "offset", 0, "Number of matching connections to skip")
	cmd.Flags().Lookup("limit").Usage = "Maximum connections to return (1–500)"
	return cmd
}
