package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type attributeLocation struct {
	Signal    string `json:"signal"`
	OwnerType string `json:"ownerType"`
}

type attributeSearchQuery struct {
	telemetrySearchQuery
	location attributeLocation
	records  string
}

type attributeKey struct {
	Key     string              `json:"key"`
	Kind    string              `json:"kind"`
	FoundOn []attributeLocation `json:"foundOn"`
}

type attributeFrequency struct {
	Value             json.RawMessage     `json:"value"`
	FoundOn           []attributeLocation `json:"foundOn"`
	Count             uint64              `json:"count"`
	Denominator       uint64              `json:"denominator"`
	RelativeFrequency float64             `json:"relativeFrequency"`
}

type attributeKeysResult struct {
	Keys      []attributeKey `json:"keys"`
	Truncated bool           `json:"truncated"`
}

type attributeValuesResult struct {
	Key       string               `json:"key"`
	Values    []attributeFrequency `json:"values"`
	Truncated bool                 `json:"truncated"`
}

func newAttributesCommand(client *http.Client, now func() time.Time) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attributes",
		Short: "🔎 Discover attribute keys and values",
		Long:  "🔎 Discover attributes in a running viewer, keeping received kinds and owner associations.",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newAttributeKeysCommand(client, now), newAttributeValuesCommand(client, now))
	return cmd
}

func newAttributeKeysCommand(client *http.Client, now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var signal, ownerType string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "🔎 List distinct attribute keys and kinds",
		Long: "🔎 List each distinct attribute key and received kind once, with its owner location. " +
			"Defaults to direct span attributes, the last hour and 25 key/kind pairs. Select the signal and owner explicitly for other locations.",
		Example: "  otel-desktop-viewer attributes keys\n" +
			"  otel-desktop-viewer attributes keys --signal logs --owner-type log --service checkout --since 30m --json\n" +
			"  otel-desktop-viewer attributes keys --signal metrics --owner-type scope --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			query, err := resolveAttributeSearch(cmd, options, signal, ownerType, now())
			if err != nil {
				return err
			}
			_, result, err := requestQuery(cmd.Context(), client, options.Endpoint, attributeKeysSQL(query), uint64(query.Limit))
			if err != nil {
				return err
			}
			keys := make([]attributeKey, 0, len(result.Rows))
			for _, row := range result.Rows {
				var key attributeKey
				if err := decodeAttributeEntry(row, &key); err != nil {
					return err
				}
				keys = append(keys, key)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(attributeKeysResult{Keys: keys, Truncated: result.Truncated})
			}
			rows := make([][]any, 0, len(keys))
			for _, key := range keys {
				rows = append(rows, []any{key.Key, key.Kind, key.FoundOn})
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatQueryColumns(queryResult{
				Columns: []queryColumn{{Name: "key"}, {Name: "kind"}, {Name: "foundOn"}},
				Rows:    rows, Truncated: result.Truncated,
			}))
			return err
		},
	}
	addAttributeFlags(cmd, &options, &signal, &ownerType, &jsonOutput)
	return cmd
}

func newAttributeValuesCommand(client *http.Client, now func() time.Time) *cobra.Command {
	options := telemetrySearchOptions{}
	var signal, ownerType string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "values <key>",
		Short: "🔎 Count exact typed values for one attribute key",
		Long: "🔎 Rank attribute values by distinct telemetry-record count: spans, logs or Metric datapoints for the selected signal. " +
			"Events/links count their owning spans; exemplars and Metric metadata count associated datapoints. Time and service filters apply to those same records. " +
			"Resource/scope values count referencing records, not distinct resources/scopes. The denominator is matching records whose selected owner carries the key, before the result limit. " +
			"A histogram datapoint counts once, not by its observation count. Kinds stay distinct. Defaults to direct span attributes, the last hour and 25 values. " +
			"Columns show value, kind, count and percentage; --json retains the tagged value, owner location, count, denominator and relative frequency. " +
			"A record with multiple values for the key contributes once to each value and once to the denominator, so percentages may sum above 100%. " +
			"Use query to find records carrying a selected typed value, then trace or span to inspect them; skills includes a checked SQL example.",
		Example: "  otel-desktop-viewer attributes values http.method\n" +
			"  otel-desktop-viewer attributes values http.method --service checkout --limit 10 --json\n" +
			"  otel-desktop-viewer attributes values region --signal metrics --owner-type resource --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			query, err := resolveAttributeSearch(cmd, options, signal, ownerType, now())
			if err != nil {
				return err
			}
			_, result, err := requestQuery(cmd.Context(), client, options.Endpoint, attributeValuesSQL(query, args[0]), uint64(query.Limit))
			if err != nil {
				return err
			}
			values := make([]attributeFrequency, 0, len(result.Rows))
			for _, row := range result.Rows {
				var value attributeFrequency
				if err := decodeAttributeEntry(row, &value); err != nil {
					return err
				}
				values = append(values, value)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(attributeValuesResult{Key: args[0], Values: values, Truncated: result.Truncated})
			}
			rows := make([][]any, 0, len(values))
			for _, value := range values {
				var tagged struct {
					Kind  string          `json:"kind"`
					Value json.RawMessage `json:"value"`
				}
				if err := json.Unmarshal(value.Value, &tagged); err != nil {
					return fmt.Errorf("decode attribute value: %w", err)
				}
				rows = append(rows, []any{tagged.Value, tagged.Kind, value.Count, strconv.FormatFloat(value.RelativeFrequency*100, 'g', -1, 64) + "%"})
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatQueryColumns(queryResult{
				Columns: []queryColumn{{Name: "value"}, {Name: "kind"}, {Name: "count"}, {Name: "percentage"}},
				Rows:    rows, Truncated: result.Truncated,
			}))
			return err
		},
	}
	addAttributeFlags(cmd, &options, &signal, &ownerType, &jsonOutput)
	return cmd
}

func addAttributeFlags(cmd *cobra.Command, options *telemetrySearchOptions, signal, ownerType *string, jsonOutput *bool) {
	addTelemetrySearchFlags(cmd, options, jsonOutput)
	cmd.Flags().Lookup("limit").Usage = "Maximum rows to return"
	cmd.Flags().Lookup("service").Usage = "Only records for this service"
	cmd.Flags().Lookup("json").Usage = "Emit exact JSON with truncation instead of columns"
	cmd.Flags().StringVar(signal, "signal", "traces", "Telemetry signal: traces, logs or metrics")
	cmd.Flags().StringVar(ownerType, "owner-type", "span", "Attribute owner: span/event/link (traces), log (logs), datapoint/exemplar/metadata (metrics), resource or scope")
}

func resolveAttributeSearch(cmd *cobra.Command, options telemetrySearchOptions, signal, ownerType string, now time.Time) (attributeSearchQuery, error) {
	records, err := attributeRecordsSQL(signal, ownerType)
	if err != nil {
		return attributeSearchQuery{}, err
	}
	query, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now)
	return attributeSearchQuery{telemetrySearchQuery: query, location: attributeLocation{Signal: signal, OwnerType: ownerType}, records: records}, err
}

func decodeAttributeEntry(row []any, entry any) error {
	if len(row) != 1 || row[0] == nil {
		return fmt.Errorf("decode attribute result: expected one non-null entry per row")
	}
	raw, err := json.Marshal(row[0])
	if err != nil {
		return fmt.Errorf("decode attribute result: %w", err)
	}
	if err := json.Unmarshal(raw, entry); err != nil {
		return fmt.Errorf("decode attribute result: %w", err)
	}
	return nil
}

// SQL text is sent through the existing read-only query method. Every caller-
// supplied string is a quoted SQL literal; identifiers are fixed here.
func attributeSQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// These projections keep the counted record identity separate from the owner
// of the attributes. Resource/scope joins do not deduplicate referencing records.
func attributeRecordsSQL(signal, ownerType string) (string, error) {
	var identity, timestamp, service, from, directOwner, attributes, resource, scope, allowed string
	switch signal {
	case "traces":
		identity, timestamp, service = "struct_pack(trace_id := s.trace_id, span_id := s.span_id)", "s.start_time", "s.service_name"
		from, directOwner, attributes, resource, scope = "spans s", "span", "s.attribute_ids", "s.resource_id", "s.scope_id"
		allowed = "span, event, link, resource or scope"
		switch ownerType {
		case "event":
			from += " JOIN events e ON e.trace_id = s.trace_id AND e.span_id = s.span_id"
			attributes, directOwner = "e.attribute_ids", "event"
		case "link":
			from += " JOIN links l ON l.trace_id = s.trace_id AND l.span_id = s.span_id"
			attributes, directOwner = "l.attribute_ids", "link"
		}
	case "logs":
		identity, timestamp, service = "l.id", "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)", "l.service_name"
		from, directOwner, attributes, resource, scope = "logs l", "log", "l.attribute_ids", "l.resource_id", "l.scope_id"
		allowed = "log, resource or scope"
	case "metrics":
		identity, timestamp, service = "d.id", "d.timestamp", "m.service_name"
		from, directOwner = "metric_datapoints d JOIN metrics m ON m.id = d.metric_id", "datapoint"
		attributes, resource, scope = "d.attribute_ids", "m.resource_id", "m.scope_id"
		allowed = "datapoint, exemplar, metadata, resource or scope"
		switch ownerType {
		case "exemplar":
			from += " JOIN exemplars e ON e.metric_datapoint_id = d.id"
			attributes, directOwner = "e.attribute_ids", "exemplar"
		case "metadata":
			attributes, directOwner = "m.metadata_ids", "metadata"
		}
	default:
		return "", fmt.Errorf("unsupported --signal %q: use traces, logs or metrics", signal)
	}
	switch ownerType {
	case directOwner:
	case "resource":
		from += " JOIN resources r ON r.id = " + resource
		attributes = "r.attribute_ids"
	case "scope":
		from += " JOIN scopes sc ON sc.id = " + scope
		attributes = "sc.attribute_ids"
	default:
		return "", fmt.Errorf("--signal %s supports --owner-type %s; got %q", signal, allowed, ownerType)
	}
	return "SELECT " + identity + " AS record_id, " + timestamp + " AS time_ns, " + service +
		" AS service_name, " + attributes + " AS attribute_ids FROM " + from, nil
}

func attributeRecordPredicate(query telemetrySearchQuery) string {
	predicates := []string{"true"}
	if query.StartTime != nil {
		predicates = append(predicates, "s.time_ns >= "+attributeSQLLiteral(*query.StartTime)+"::UBIGINT")
	}
	if query.EndTime != nil {
		predicates = append(predicates, "s.time_ns <= "+attributeSQLLiteral(*query.EndTime)+"::UBIGINT")
	}
	if query.Service != "" {
		predicates = append(predicates, "s.service_name = "+attributeSQLLiteral(query.Service))
	}
	return strings.Join(predicates, " AND ")
}

func attributeFoundOnSQL(location attributeLocation) string {
	return "json_array(json_object('signal', " + attributeSQLLiteral(location.Signal) + ", 'ownerType', " + attributeSQLLiteral(location.OwnerType) + "))"
}

func attributeKeysSQL(query attributeSearchQuery) string {
	return `WITH records AS (` + query.records + `)
SELECT json_object('key', a.key, 'kind', json_extract_string(a.value, '$.kind'),
    'foundOn', ` + attributeFoundOnSQL(query.location) + `) AS entry
FROM records s
CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
JOIN attributes a ON a.id = owned.attribute_id
WHERE ` + attributeRecordPredicate(query.telemetrySearchQuery) + `
GROUP BY a.key, json_extract_string(a.value, '$.kind')
ORDER BY a.key, json_extract_string(a.value, '$.kind')`
}

func attributeValuesSQL(query attributeSearchQuery, key string) string {
	return `WITH records AS (` + query.records + `), owned_values AS (
    SELECT s.record_id, a.value
    FROM records s
    CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
    JOIN attributes a ON a.id = owned.attribute_id
    WHERE ` + attributeRecordPredicate(query.telemetrySearchQuery) + ` AND a.key = ` + attributeSQLLiteral(key) + `
), value_counts AS (
    SELECT value, count(DISTINCT record_id) AS count
    FROM owned_values GROUP BY value
), denominator AS (
    SELECT count(DISTINCT record_id) AS count
    FROM owned_values
)
SELECT json_object('value', v.value,
    'foundOn', ` + attributeFoundOnSQL(query.location) + `,
    'count', v.count, 'denominator', d.count,
    'relativeFrequency', v.count::DOUBLE / nullif(d.count, 0)::DOUBLE) AS entry
FROM value_counts v CROSS JOIN denominator d
ORDER BY v.count DESC, json_extract_string(v.value, '$.kind'), v.value::VARCHAR`
}
