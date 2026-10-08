package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func newMetricCommand(client *http.Client) *cobra.Command {
	var endpoint, seriesRef, start, end string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "metric <metric-ref>",
		Short: "📈 Inspect one Metric or its series datapoints",
		Long: "📈 Inspect a Metric descriptor, metadata, resource, scope and series catalogue. " +
			"Use metricRef from metrics --json. Select --series to retrieve its exact retained datapoints, optionally bounded by --start/--end. " +
			"Unspecified bounds include all retained times; all selected datapoints are returned. " +
			"metricRef, seriesRef and datapointRef are viewer-generated database references. " +
			"Catalogue counts and first/last timestamps are computed from stored datapoints. " +
			"--json preserves the detail response: 64-bit integers and nanosecond timestamps use decimal strings; doubles use JSON numbers or IEEE-754 bit strings for special values and negative zero.",
		Example: "  otel-desktop-viewer metric 018f0000-0000-7000-8000-000000000001\n" +
			"  otel-desktop-viewer metric 018f0000-0000-7000-8000-000000000001 --series 018f0000-0000-7000-8000-000000000002 --start 2026-10-07T08:00:00Z --end 2026-10-07T09:00:00Z --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			ref, err := normalizeDetailRef(args[0], "metric")
			if err != nil {
				return err
			}
			if seriesRef == "" && (cmd.Flags().Changed("series") || cmd.Flags().Changed("start") || cmd.Flags().Changed("end")) {
				return fmt.Errorf("--start and --end require a nonempty --series reference")
			}
			method := "getMetric"
			params := map[string]any{"metricRef": ref}
			if seriesRef != "" {
				series, err := normalizeDetailRef(seriesRef, "series")
				if err != nil {
					return err
				}
				bounds := map[string]*uint64{"start": nil, "end": nil}
				for _, bound := range []struct{ flag, value string }{{"start", start}, {"end", end}} {
					if cmd.Flags().Changed(bound.flag) {
						value, err := parseTelemetryTime(bound.value, "--"+bound.flag)
						if err != nil {
							return err
						}
						bounds[bound.flag] = value
					}
				}
				if bounds["start"] != nil && bounds["end"] != nil && *bounds["start"] > *bounds["end"] {
					return fmt.Errorf("--start must not be after --end")
				}
				method = "getMetricSeries"
				params["seriesRef"] = series
				for _, flag := range []string{"start", "end"} {
					params[flag+"Time"] = nil
					if bounds[flag] != nil {
						params[flag+"Time"] = telemetryDecimalString(*bounds[flag])
					}
				}
			}
			raw, err := requestViewerRPC(cmd.Context(), client, endpoint, method, params)
			if err != nil {
				return err
			}
			if jsonOutput {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return err
			}
			formatted, err := formatMetricDetail(raw, seriesRef != "")
			if err != nil {
				return fmt.Errorf("decode viewer %s result: %w", method, err)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatted)
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().StringVar(&seriesRef, "series", "", "Series reference from the Metric catalogue; return its exact datapoints")
	cmd.Flags().StringVar(&start, "start", "", "Inclusive RFC3339 datapoint start time; requires --series")
	cmd.Flags().StringVar(&end, "end", "", "Inclusive RFC3339 datapoint end time; requires --series")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the complete detail response as JSON instead of sections")
	return cmd
}

func formatMetricDetail(raw json.RawMessage, seriesSelected bool) (string, error) {
	var metric struct {
		MetricRef                  string       `json:"metricRef"`
		Name                       string       `json:"name"`
		Description                string       `json:"description"`
		Unit                       string       `json:"unit"`
		MetricType                 string       `json:"metricType"`
		AggregationTemporalityCode *json.Number `json:"aggregationTemporalityCode"`
		IsMonotonic                *bool        `json:"isMonotonic"`
		Metadata                   []detailAttr `json:"metadata"`
		Resource                   struct {
			detailResource
			SchemaURL string `json:"schemaUrl"`
		} `json:"resource"`
		Scope struct {
			detailScope
			SchemaURL string `json:"schemaUrl"`
		} `json:"scope"`
		Series     []json.RawMessage `json:"series"`
		SeriesRef  string            `json:"seriesRef"`
		Attributes []detailAttr      `json:"attributes"`
		Datapoints []json.RawMessage `json:"datapoints"`
	}
	if err := decodeExactJSON(raw, &metric); err != nil {
		return "", err
	}
	if metric.MetricRef == "" || metric.MetricType == "" {
		return "", fmt.Errorf("missing metricRef or metricType")
	}
	fields := []string{"metricRef", "name", "description", "unit", "metricType"}
	values := []any{metric.MetricRef, metric.Name, metric.Description, metric.Unit, metric.MetricType}
	if metric.AggregationTemporalityCode != nil {
		fields = append(fields, "aggregationTemporalityCode")
		values = append(values, *metric.AggregationTemporalityCode)
	}
	if metric.IsMonotonic != nil {
		fields = append(fields, "isMonotonic")
		values = append(values, *metric.IsMonotonic)
	}
	output := "METRIC\n" + detailTable(fields, [][]any{values}) +
		"\nMETADATA\n" + formatAttributes(metric.Metadata) +
		"\nRESOURCE\n" + detailTable([]string{"schemaURL", "droppedAttributesCount"}, [][]any{{metric.Resource.SchemaURL, metric.Resource.DroppedAttributesCount}}) +
		"\nRESOURCE ATTRIBUTES\n" + formatAttributes(metric.Resource.Attributes) +
		"\nSCOPE\n" + detailTable([]string{"name", "version", "schemaURL", "droppedAttributesCount"}, [][]any{{metric.Scope.Name, metric.Scope.Version, metric.Scope.SchemaURL, metric.Scope.DroppedAttributesCount}}) +
		"\nSCOPE ATTRIBUTES\n" + formatAttributes(metric.Scope.Attributes)
	if !seriesSelected {
		if metric.Series == nil {
			return "", fmt.Errorf("missing series catalogue")
		}
		rows, err := decodeTelemetryRows(metric.Series, []string{"seriesRef", "attributes", "datapointCount", "firstDatapointTimestamp", "lastDatapointTimestamp"})
		if err != nil {
			return "", err
		}
		return output + fmt.Sprintf("\nSERIES (%d; counts and timestamp bounds are computed)\n", len(rows)) +
			detailTable([]string{"seriesRef", "attributes", "datapointCount", "firstDatapointTimestamp", "lastDatapointTimestamp"}, rows), nil
	}
	if metric.SeriesRef == "" || metric.Datapoints == nil {
		return "", fmt.Errorf("missing seriesRef or datapoints")
	}
	columns := []string{"datapointRef", "timestamp", "startTime", "flags"}
	switch metric.MetricType {
	case "Gauge", "Sum":
		columns = append(columns, "valueType", "intValue", "doubleValue")
	case "Histogram":
		columns = append(columns, "count", "sum", "min", "max", "bucketCounts", "explicitBounds")
	case "ExponentialHistogram":
		columns = append(columns, "count", "sum", "min", "max", "scale", "zeroCount", "zeroThreshold", "positive", "negative")
	default:
		return "", fmt.Errorf("unsupported Metric type %q", metric.MetricType)
	}
	columns = append(columns, "exemplars")
	rows := make([][]any, 0, len(metric.Datapoints))
	for _, raw := range metric.Datapoints {
		var point map[string]any
		if err := decodeExactJSON(raw, &point); err != nil {
			return "", err
		}
		row := make([]any, len(columns))
		for i, column := range columns {
			value, ok := point[column]
			if !ok {
				if column != "sum" && column != "min" && column != "max" {
					return "", fmt.Errorf("datapoint is missing %s", column)
				}
				value = "(absent)"
			}
			row[i] = value
		}
		rows = append(rows, row)
	}
	return output + "\nSERIES\n" + detailTable([]string{"seriesRef"}, [][]any{{metric.SeriesRef}}) +
		"\nDATAPOINT ATTRIBUTES\n" + formatAttributes(metric.Attributes) +
		fmt.Sprintf("\nDATAPOINTS (%d; special doubles and negative zero use IEEE-754 bit strings)\n", len(rows)) + detailTable(columns, rows), nil
}
