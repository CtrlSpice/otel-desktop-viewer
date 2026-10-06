package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type traceDetailResult struct {
	Trace traceDetailTrace `json:"trace"`
	Logs  []traceDetailLog `json:"logs"`
}

type traceDetailTrace struct {
	TraceID    string                         `json:"traceID"`
	TraceStart string                         `json:"traceStart"`
	Resources  map[string]traceDetailResource `json:"resources"`
	Spans      []traceDetailSpan              `json:"spans"`
}

type traceDetailResource struct {
	Attributes []traceDetailAttribute `json:"attributes"`
}

type traceDetailAttribute struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type traceDetailSpan struct {
	SpanData traceDetailSpanData `json:"spanData"`
}

type traceDetailSpanData struct {
	SpanID       string          `json:"spanID"`
	ParentSpanID *string         `json:"parentSpanID"`
	Name         string          `json:"name"`
	KindCode     json.Number     `json:"kindCode"`
	Kind         string          `json:"kind"`
	Start        string          `json:"start"`
	Duration     string          `json:"dur"`
	Resource     json.RawMessage `json:"r"`
	StatusCode   string          `json:"statusCode"`
}

type traceDetailLog struct {
	Timestamp         string              `json:"timestamp"`
	ObservedTimestamp string              `json:"observedTimestamp"`
	SpanID            *string             `json:"spanID"`
	SeverityText      string              `json:"severityText"`
	SeverityNumber    json.Number         `json:"severityNumber"`
	Body              json.RawMessage     `json:"body"`
	Resource          traceDetailResource `json:"resource"`
	EventName         string              `json:"eventName"`
}

func newTraceCommand(client *http.Client) *cobra.Command {
	var endpoint string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "trace <trace-id>",
		Short: "🧵 Inspect one complete trace",
		Long: "🧵 Inspect every stored span and trace-linked log for one trace. " +
			"Default output uses compact aligned sections; --json emits the complete stored detail, including attributes, events, links, resources, scopes, and schema URLs. " +
			"Trace start is the minimum stored span start; trace duration is maximum span end minus minimum span start; " +
			"span start offset is stored span start minus the minimum stored span start; span duration is stored end minus stored start. These values are exact nanoseconds. " +
			"A displayed log timestamp uses its stored timestamp when non-zero, otherwise its stored observed timestamp.",
		Example: "  otel-desktop-viewer trace 0123456789abcdef0123456789abcdef\n" +
			"  otel-desktop-viewer trace 01234567-89ab-cdef-0123-456789abcdef --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			traceID, err := normalizeTraceID(args[0])
			if err != nil {
				return err
			}
			raw, err := requestViewerRPC(cmd.Context(), client, endpoint, "getTraceDetail", map[string]any{"traceID": traceID})
			if err != nil {
				return err
			}
			if jsonOutput {
				if _, err := cmd.OutOrStdout().Write(raw); err != nil {
					return err
				}
				_, err = io.WriteString(cmd.OutOrStdout(), "\n")
				return err
			}
			result, err := decodeTraceDetail(raw)
			if err != nil {
				return fmt.Errorf("decode viewer getTraceDetail result: %w", err)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatTraceDetail(result))
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the complete trace detail as JSON instead of sections")
	return cmd
}

func normalizeTraceID(value string) (string, error) {
	if len(value) != 32 && len(value) != 36 {
		return "", fmt.Errorf("invalid trace ID %q: expected 32-char hex or a dashed UUID", value)
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid trace ID %q: expected 32-char hex or a dashed UUID", value)
	}
	return strings.ReplaceAll(id.String(), "-", ""), nil
}

func decodeTraceDetail(raw json.RawMessage) (traceDetailResult, error) {
	var result traceDetailResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return traceDetailResult{}, err
	}
	if result.Logs == nil {
		result.Logs = []traceDetailLog{}
	}
	return result, nil
}

func formatTraceDetail(result traceDetailResult) string {
	spanRows := make([][]any, len(result.Trace.Spans))
	var traceEnd *big.Int
	for i, span := range result.Trace.Spans {
		start := addDecimal(result.Trace.TraceStart, span.SpanData.Start)
		end := addDecimal(start, span.SpanData.Duration)
		if parsed, ok := new(big.Int).SetString(end, 10); ok && (traceEnd == nil || parsed.Cmp(traceEnd) > 0) {
			traceEnd = parsed
		}
		spanRows[i] = []any{span.SpanData.SpanID, nullableString(span.SpanData.ParentSpanID),
			resourceService(result.Trace.Resources, span.SpanData.Resource), span.SpanData.Name,
			span.SpanData.Start, span.SpanData.Duration}
	}
	duration := "0"
	if start, ok := new(big.Int).SetString(result.Trace.TraceStart, 10); ok && traceEnd != nil {
		duration = new(big.Int).Sub(traceEnd, start).String()
	}

	logRows := make([][]any, len(result.Logs))
	for i, log := range result.Logs {
		logRows[i] = []any{effectiveLogTimestamp(log.Timestamp, log.ObservedTimestamp), nullableString(log.SpanID),
			log.SeverityText, strconv.FormatInt(mustJSONInt(log.SeverityNumber), 10),
			attributeService(log.Resource.Attributes), log.EventName, compactTraceTaggedValue(log.Body)}
	}

	traceTable := formatQueryColumns(queryResult{Columns: traceColumns("traceID", "spans", "logs", "startTime", "durationNs"), Rows: [][]any{{
		result.Trace.TraceID, len(result.Trace.Spans), len(result.Logs), result.Trace.TraceStart, duration,
	}}})
	spanTable := formatQueryColumns(queryResult{Columns: traceColumns("spanID", "parentSpanID", "service", "name", "startOffsetNs", "durationNs"), Rows: spanRows})
	logTable := formatQueryColumns(queryResult{Columns: traceColumns("timestamp", "spanID", "severity", "severityNumber", "service", "eventName", "body"), Rows: logRows})
	return "TRACE\n" + traceTable + "\nSPANS\n" + spanTable + "\nTRACE LOGS\n" + logTable
}

func traceColumns(names ...string) []queryColumn {
	columns := make([]queryColumn, len(names))
	for i, name := range names {
		columns[i] = queryColumn{Name: name}
	}
	return columns
}

func addDecimal(left, right string) string {
	a, aOK := new(big.Int).SetString(left, 10)
	b, bOK := new(big.Int).SetString(right, 10)
	if !aOK || !bOK {
		return "0"
	}
	return new(big.Int).Add(a, b).String()
}

func effectiveLogTimestamp(timestamp, observed string) string {
	if timestamp != "0" {
		return timestamp
	}
	return observed
}

func resourceService(resources map[string]traceDetailResource, raw json.RawMessage) string {
	var key json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&key) != nil {
		return ""
	}
	return attributeService(resources[key.String()].Attributes)
}

func attributeService(attributes []traceDetailAttribute) string {
	for _, attribute := range attributes {
		if attribute.Key == "service.name" {
			return compactTraceTaggedValue(attribute.Value)
		}
	}
	return ""
}

func compactTraceTaggedValue(raw json.RawMessage) string {
	var value struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Kind == "" {
		return string(raw)
	}
	if value.Kind == "string" {
		var text string
		if json.Unmarshal(value.Value, &text) == nil {
			return text
		}
	}
	return string(value.Value)
}

func mustJSONInt(value json.Number) int64 {
	parsed, _ := value.Int64()
	return parsed
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
