package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/spf13/cobra"
)

type attributeLocation struct {
	Signal    string `json:"signal"`
	OwnerType string `json:"ownerType"`
}

type attributeSearchQuery struct {
	telemetrySearchQuery
	builder discovery.Builder
	records string
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
		Short: "🏷️ Discover attribute keys and values",
		Long:  "🏷️ Discover attributes in a running viewer, keeping received kinds and owner associations.",
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
		Short: "🔑 List distinct attribute keys and kinds",
		Long: "🔑 List each distinct attribute key and received kind once, with the locations where it occurs. " +
			"Searches all signals and owners in the last hour and returns up to 25 key/kind pairs. Narrow by signal, owner or both.",
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
		Short: "🏷️ Count exact typed values for one attribute key",
		Long: "🏷️ Rank attribute values by distinct telemetry-record count: spans, logs and Metric datapoints across the selected locations. " +
			"Events/links count their owning spans; exemplars and Metric metadata count associated datapoints. Events and exemplars use their own timestamps; other owners use the counted record's time. Service filters follow the associated record. " +
			"Resource/scope values count referencing records, not distinct resources/scopes. The denominator is matching records whose selected owner carries the key, before the result limit. " +
			"A histogram datapoint counts once, not by its observation count. Kinds stay distinct. Searches all signals and owners in the last hour and returns up to 25 values. " +
			"Columns show value, kind, count, percentage and owner locations; --json retains the tagged value, owner location, count, denominator and relative frequency. " +
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
				rows = append(rows, []any{tagged.Value, tagged.Kind, value.Count, strconv.FormatFloat(value.RelativeFrequency*100, 'f', 2, 64) + "%", value.FoundOn})
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatQueryColumns(queryResult{
				Columns: []queryColumn{{Name: "value"}, {Name: "kind"}, {Name: "count"}, {Name: "percentage"}, {Name: "foundOn"}},
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
	cmd.Flags().StringVar(signal, "signal", "", "Limit to one signal: traces, logs or metrics")
	cmd.Flags().StringVar(ownerType, "owner-type", "", "Limit to one owner: span/event/link (traces), log (logs), datapoint/exemplar/metadata (metrics), resource or scope")
}

func resolveAttributeSearch(cmd *cobra.Command, options telemetrySearchOptions, signal, ownerType string, now time.Time) (attributeSearchQuery, error) {
	query, err := resolveTelemetrySearch(options, cmd.Flags().Changed("since"), now)
	if err != nil {
		return attributeSearchQuery{}, err
	}
	var b discovery.Builder
	records, err := b.AttributeRecords(discoveryQuery(query), signal, ownerType)
	return attributeSearchQuery{telemetrySearchQuery: query, builder: b, records: records}, err
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

func discoveryQuery(query telemetrySearchQuery) discovery.Query {
	return discovery.Query{StartTime: query.StartTime, EndTime: query.EndTime, Service: query.Service}
}

func attributeKeysSQL(query attributeSearchQuery) string {
	return query.builder.Inline(query.builder.AttributeKeys(discoveryQuery(query.telemetrySearchQuery), query.records))
}

func attributeValuesSQL(query attributeSearchQuery, key string) string {
	return query.builder.Inline(query.builder.AttributeValues(discoveryQuery(query.telemetrySearchQuery), query.records, key))
}
