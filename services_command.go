package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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
			"Group by Resource service.namespace and service.name, ordered by namespace then name. " +
			"Missing and empty namespaces share a group; missing names display as empty strings. " +
			"Names and namespaces are derived text labels; original typed Resource attributes remain available for inspection. Values with the same text share a summary. " +
			"Counts cover stored records in the selected window, not requests. Error spans have status Error; error logs have numeric severity ERROR or FATAL (17–24). " +
			"Metrics counts exact Metric identities with datapoints in the window; a histogram datapoint counts once. " +
			"Time filtering and lastSeen use span start, log timestamp (observed timestamp when timestamp is zero), or datapoint timestamp. " +
			"Timestamps are exact decimal Unix nanoseconds. Defaults to the last hour and 25 services. " +
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
	predicate := attributeRecordPredicate(query)
	return `WITH resource_counts AS (
    SELECT resource_id, service_name, count(*) AS span_count,
        count(*) FILTER (WHERE status_code = 2) AS error_span_count,
        0 AS log_count, 0 AS error_log_count, 0 AS metric_count, 0 AS data_point_count,
        max(time_ns) AS last_seen
    FROM (SELECT resource_id, service_name, start_time AS time_ns, status_code FROM spans) s
    WHERE ` + predicate + ` GROUP BY resource_id, service_name
    UNION ALL
    SELECT resource_id, service_name, 0, 0, count(*),
        count(*) FILTER (WHERE severity_number BETWEEN 17 AND 24), 0, 0, max(time_ns)
    FROM (SELECT resource_id, service_name,
        coalesce(nullif(timestamp, 0), observed_timestamp) AS time_ns, severity_number FROM logs) s
    WHERE ` + predicate + ` GROUP BY resource_id, service_name
    UNION ALL
    SELECT resource_id, service_name, 0, 0, 0, 0, count(DISTINCT metric_id), count(*), max(time_ns)
    FROM (SELECT m.resource_id, m.service_name, m.id AS metric_id, d.timestamp AS time_ns
        FROM metric_datapoints d JOIN metrics m ON m.id = d.metric_id) s
    WHERE ` + predicate + ` GROUP BY resource_id, service_name
), identities AS (
    SELECT r.id,
        coalesce((SELECT attribute_text(a.value)
            FROM attributes a WHERE a.key = 'service.namespace' AND list_contains(r.attribute_ids, a.id)), '') AS service_namespace
    FROM resources r JOIN (SELECT DISTINCT resource_id FROM resource_counts) used ON used.resource_id = r.id
)
SELECT json_object(
    'serviceNamespace', i.service_namespace, 'serviceName', c.service_name,
    'spanCount', sum(c.span_count), 'errorSpanCount', sum(c.error_span_count),
    'logCount', sum(c.log_count), 'errorLogCount', sum(c.error_log_count),
    'metricCount', sum(c.metric_count), 'dataPointCount', sum(c.data_point_count),
    'lastSeen', max(c.last_seen)::VARCHAR) AS entry
FROM resource_counts c JOIN identities i ON i.id = c.resource_id
GROUP BY i.service_namespace, c.service_name
ORDER BY i.service_namespace, c.service_name`
}
