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
		Short: "🔎 Discover span attribute keys and values",
		Long:  "🔎 Discover direct span attributes in a running viewer, keeping received kinds and owner associations.",
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
		Short: "🔎 List distinct span attribute keys and kinds",
		Long: "🔎 List each distinct direct span attribute key and received kind once, with its owner location. " +
			"Defaults to the last hour and 25 key/kind pairs. Resource, scope, event and link attributes are excluded.",
		Example: "  otel-desktop-viewer attributes keys\n" +
			"  otel-desktop-viewer attributes keys --signal traces --owner-type span --service checkout --since 30m --json",
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
		Short: "🔎 Count exact typed values for one span attribute key",
		Long: "🔎 Rank direct span attribute values by distinct owning-span count. " +
			"The denominator is all matching spans carrying the exact key, before the result limit. " +
			"Kinds stay distinct. Defaults to the last hour and 25 values. " +
			"Columns show value, kind, count and percentage; --json retains the tagged value, owner location, count, denominator and relative frequency. " +
			"A span with multiple values for the key contributes once to each value and once to the denominator, so percentages may sum above 100%.",
		Example: "  otel-desktop-viewer attributes values http.method\n" +
			"  otel-desktop-viewer attributes values http.method --service checkout --limit 10 --json",
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
	cmd.Flags().Lookup("service").Usage = "Only spans for this service"
	cmd.Flags().Lookup("json").Usage = "Emit exact JSON with truncation instead of columns"
	cmd.Flags().StringVar(signal, "signal", "traces", "Telemetry signal (traces supported)")
	cmd.Flags().StringVar(ownerType, "owner-type", "span", "Attribute owner (span supported)")
}

func resolveAttributeSearch(cmd *cobra.Command, options telemetrySearchOptions, signal, ownerType string, now time.Time) (telemetrySearchQuery, error) {
	if signal != "traces" || ownerType != "span" {
		return telemetrySearchQuery{}, fmt.Errorf("attribute commands currently support --signal traces --owner-type span")
	}
	return resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now)
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

func attributeSpanPredicate(query telemetrySearchQuery) string {
	predicates := []string{"true"}
	if query.StartTime != nil {
		predicates = append(predicates, "s.start_time >= "+attributeSQLLiteral(*query.StartTime)+"::UBIGINT")
	}
	if query.EndTime != nil {
		predicates = append(predicates, "s.start_time <= "+attributeSQLLiteral(*query.EndTime)+"::UBIGINT")
	}
	if query.Service != "" {
		predicates = append(predicates, "s.service_name = "+attributeSQLLiteral(query.Service))
	}
	return strings.Join(predicates, " AND ")
}

func attributeKeysSQL(query telemetrySearchQuery) string {
	return `SELECT json_object('key', a.key, 'kind', json_extract_string(a.value, '$.kind'),
    'foundOn', json('[{"signal":"traces","ownerType":"span"}]')) AS entry
FROM spans s
CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
JOIN attributes a ON a.id = owned.attribute_id
WHERE ` + attributeSpanPredicate(query) + `
GROUP BY a.key, json_extract_string(a.value, '$.kind')
ORDER BY a.key, json_extract_string(a.value, '$.kind')`
}

func attributeValuesSQL(query telemetrySearchQuery, key string) string {
	return `WITH owned_values AS (
    SELECT s.trace_id, s.span_id, a.value
    FROM spans s
    CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
    JOIN attributes a ON a.id = owned.attribute_id
    WHERE ` + attributeSpanPredicate(query) + ` AND a.key = ` + attributeSQLLiteral(key) + `
), value_counts AS (
    SELECT value, count(DISTINCT struct_pack(trace_id := trace_id, span_id := span_id)) AS count
    FROM owned_values GROUP BY value
), denominator AS (
    SELECT count(DISTINCT struct_pack(trace_id := trace_id, span_id := span_id)) AS count
    FROM owned_values
)
SELECT json_object('value', v.value,
    'foundOn', json('[{"signal":"traces","ownerType":"span"}]'),
    'count', v.count, 'denominator', d.count,
    'relativeFrequency', v.count::DOUBLE / nullif(d.count, 0)::DOUBLE) AS entry
FROM value_counts v CROSS JOIN denominator d
ORDER BY v.count DESC, json_extract_string(v.value, '$.kind'), v.value::VARCHAR`
}
