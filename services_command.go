package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/spf13/cobra"
)

type serviceSummary struct {
	ServiceNamespace string `json:"serviceNamespace"`
	ServiceName      string `json:"serviceName"`
	SpanCount        uint64 `json:"spanCount"`
	ErrorSpanCount   uint64 `json:"errorSpanCount"`
	LogCount         uint64 `json:"logCount"`
	ErrorLogCount    uint64 `json:"errorLogCount"`
	MetricCount      uint64 `json:"metricCount"`
	DataPointCount   uint64 `json:"dataPointCount"`
	LastSeen         string `json:"lastSeen"`
}

type servicesResult struct {
	StartTime *string          `json:"startTime"`
	EndTime   *string          `json:"endTime"`
	Services  []serviceSummary `json:"services"`
	Truncated bool             `json:"truncated"`
}

func newServicesCommand(client *http.Client, now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Discover services and their telemetry counts",
		Long: "Discover services with spans, logs or Metric datapoints in a running viewer. " +
			"Group by service.namespace and service.name, ordered by namespace then name. " +
			"Show span/error-span, log/error-log and Metric/datapoint counts, plus lastSeen in Unix nanoseconds. " +
			"Counts describe stored telemetry, not requests or service health. Defaults to the last hour and 25 services. " +
			"--service matches an exact name across namespaces. --json includes window bounds and truncation.",
		Example: "  otel-desktop-viewer services\n" +
			"  otel-desktop-viewer services --service checkout --since 30m\n" +
			"  otel-desktop-viewer services --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			query, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now())
			if err != nil {
				return err
			}
			_, result, err := requestQuery(cmd.Context(), client, options.Endpoint, servicesSQL(query), uint64(query.Limit))
			if err != nil {
				return err
			}
			services := make([]serviceSummary, 0, len(result.Rows))
			for _, row := range result.Rows {
				if len(row) != 1 || row[0] == nil {
					return fmt.Errorf("decode service summary: expected one non-null entry per row")
				}
				raw, err := json.Marshal(row[0])
				if err != nil {
					return fmt.Errorf("decode service summary: %w", err)
				}
				var summary serviceSummary
				if err := json.Unmarshal(raw, &summary); err != nil {
					return fmt.Errorf("decode service summary: %w", err)
				}
				services = append(services, summary)
			}
			return writeServicesResult(cmd.OutOrStdout(), servicesResult{
				StartTime: query.StartTime, EndTime: query.EndTime,
				Services: services, Truncated: result.Truncated,
			}, jsonOutput)
		},
	}
	addTelemetrySearchFlags(cmd, &options, &jsonOutput)
	cmd.Flags().Lookup("service").Usage = "Only services with this exact name, across namespaces"
	cmd.Flags().Lookup("limit").Usage = "Maximum services to return"
	cmd.Flags().Lookup("json").Usage = "Emit JSON with window bounds and truncation instead of columns"
	return cmd
}

func writeServicesResult(writer io.Writer, result servicesResult, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(writer).Encode(result)
	}
	rows := make([][]any, 0, len(result.Services))
	for _, service := range result.Services {
		rows = append(rows, []any{service.ServiceNamespace, service.ServiceName,
			service.SpanCount, service.ErrorSpanCount, service.LogCount, service.ErrorLogCount,
			service.MetricCount, service.DataPointCount, service.LastSeen})
	}
	_, err := io.WriteString(writer, formatQueryColumns(queryResult{
		Columns: []queryColumn{
			{Name: "serviceNamespace"}, {Name: "serviceName"},
			{Name: "spanCount"}, {Name: "errorSpanCount"}, {Name: "logCount"}, {Name: "errorLogCount"},
			{Name: "metricCount"}, {Name: "dataPointCount"}, {Name: "lastSeen"},
		},
		Rows: rows, Truncated: result.Truncated,
	}))
	return err
}

// Aggregate each signal independently before joining Resource attributes. This
// prevents cross-signal joins from multiplying counts and counts a histogram
// datapoint once, irrespective of its received observation count. Original
// Resource attributes retain their received types. Discovery groups the existing
// service_name text projection and the equivalent namespace text projection.
// Only resources with matching records participate in discovery.
func servicesSQL(query telemetrySearchQuery) string {
	return discovery.Services(discoveryQuery(query))
}
