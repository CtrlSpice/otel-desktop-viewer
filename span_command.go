package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type spanCommandResult struct {
	Status     string          `json:"status"`
	SpanID     string          `json:"spanID"`
	TraceID    *string         `json:"traceID"`
	MatchCount int64           `json:"matchCount"`
	Summaries  json.RawMessage `json:"summaries"`
	Truncated  bool            `json:"truncated"`
	Span       json.RawMessage `json:"span"`
	Logs       json.RawMessage `json:"logs"`
}

var spanSummaryFields = []string{"traceID", "spanID", "parentSpanID", "service", "name", "startTime", "durationNs"}

type spanDetail struct {
	TraceID                string         `json:"traceID"`
	TraceState             string         `json:"traceState"`
	SpanID                 string         `json:"spanID"`
	ParentSpanID           *string        `json:"parentSpanID"`
	Flags                  json.Number    `json:"flags"`
	Name                   string         `json:"name"`
	KindCode               json.Number    `json:"kindCode"`
	Kind                   string         `json:"kind"`
	StartTime              string         `json:"startTime"`
	EndTime                string         `json:"endTime"`
	Attributes             []detailAttr   `json:"attributes"`
	Events                 []spanEvent    `json:"events"`
	Links                  []spanLink     `json:"links"`
	Resource               detailResource `json:"resource"`
	Scope                  detailScope    `json:"scope"`
	ResourceSchemaURL      string         `json:"resourceSchemaURL"`
	ScopeSchemaURL         string         `json:"scopeSchemaURL"`
	DroppedAttributesCount json.Number    `json:"droppedAttributesCount"`
	DroppedEventsCount     json.Number    `json:"droppedEventsCount"`
	DroppedLinksCount      json.Number    `json:"droppedLinksCount"`
	StatusCodeValue        json.Number    `json:"statusCodeValue"`
	StatusCode             string         `json:"statusCode"`
	StatusMessage          string         `json:"statusMessage"`
}

type detailAttr struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type detailResource struct {
	Attributes             []detailAttr `json:"attributes"`
	DroppedAttributesCount json.Number  `json:"droppedAttributesCount"`
}

type detailScope struct {
	Name                   string       `json:"name"`
	Version                string       `json:"version"`
	Attributes             []detailAttr `json:"attributes"`
	DroppedAttributesCount json.Number  `json:"droppedAttributesCount"`
}

type spanEvent struct {
	Name                   string       `json:"name"`
	Timestamp              string       `json:"timestamp"`
	DroppedAttributesCount json.Number  `json:"droppedAttributesCount"`
	Attributes             []detailAttr `json:"attributes"`
}

type spanLink struct {
	TraceID                *string      `json:"traceID"`
	SpanID                 *string      `json:"spanID"`
	TraceState             string       `json:"traceState"`
	DroppedAttributesCount json.Number  `json:"droppedAttributesCount"`
	Flags                  json.Number  `json:"flags"`
	Attributes             []detailAttr `json:"attributes"`
}

type spanLog struct {
	ID                     string          `json:"id"`
	Timestamp              string          `json:"timestamp"`
	ObservedTimestamp      string          `json:"observedTimestamp"`
	TraceID                string          `json:"traceID"`
	SpanID                 *string         `json:"spanID"`
	SeverityText           string          `json:"severityText"`
	SeverityNumber         json.Number     `json:"severityNumber"`
	Body                   json.RawMessage `json:"body"`
	Resource               detailResource  `json:"resource"`
	Scope                  detailScope     `json:"scope"`
	ResourceSchemaURL      string          `json:"resourceSchemaURL"`
	ScopeSchemaURL         string          `json:"scopeSchemaURL"`
	DroppedAttributesCount json.Number     `json:"droppedAttributesCount"`
	Flags                  json.Number     `json:"flags"`
	EventName              string          `json:"eventName"`
	Attributes             []detailAttr    `json:"attributes"`
}

func newSpanCommand(client *http.Client) *cobra.Command {
	var endpoint string
	var jsonOutput bool
	var limit int64
	cmd := &cobra.Command{
		Use:   "span <span-id> | <trace-id> <span-id>",
		Short: "🧵 Inspect one span",
		Long:  "🧵 Inspect one span and its exactly correlated logs. A standalone ambiguous ID returns bounded span summaries; it never guesses.",
		Example: "  otel-desktop-viewer span 000000000000002a\n" +
			"  otel-desktop-viewer span 0123456789abcdef0123456789abcdef 000000000000002a --json",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			if limit < 1 {
				return errors.New("--limit must be greater than zero")
			}
			if limit == math.MaxInt64 {
				return errors.New("--limit is too large")
			}

			var traceID string
			spanArg := args[0]
			if len(args) == 2 {
				var err error
				traceID, err = normalizeTraceID(args[0])
				if err != nil {
					return err
				}
				spanArg = args[1]
			}
			spanID, err := normalizeSpanID(spanArg)
			if err != nil {
				return err
			}
			params := map[string]any{"spanID": spanID}
			if traceID != "" {
				params["traceID"] = traceID
			} else {
				params["limit"] = limit
			}
			raw, err := requestViewerRPC(cmd.Context(), client, endpoint, "getSpan", params)
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
			result, err := decodeSpanCommandResult(raw)
			if err != nil {
				return fmt.Errorf("decode viewer getSpan result: %w", err)
			}
			formatted, err := formatSpanCommandResult(result)
			if err != nil {
				return fmt.Errorf("format viewer getSpan result: %w", err)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatted)
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the exact span result as JSON instead of sections")
	cmd.Flags().Int64Var(&limit, "limit", telemetryDefaultLimit, "Maximum summaries to return when a span ID is ambiguous")
	return cmd
}

func normalizeSpanID(value string) (string, error) {
	if len(value) != 16 {
		return "", errors.New("invalid span ID: expected exactly 16 hexadecimal digits")
	}
	if _, err := strconv.ParseUint(value, 16, 64); err != nil {
		return "", errors.New("invalid span ID: expected exactly 16 hexadecimal digits")
	}
	return strings.ToLower(value), nil
}

func decodeSpanCommandResult(raw json.RawMessage) (spanCommandResult, error) {
	var result spanCommandResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return spanCommandResult{}, err
	}
	switch result.Status {
	case "notFound":
		if result.SpanID == "" {
			return spanCommandResult{}, errors.New("notFound result is missing spanID")
		}
	case "ambiguous":
		var summaries []json.RawMessage
		if result.SpanID == "" || result.MatchCount < 2 || len(result.Summaries) == 0 || json.Unmarshal(result.Summaries, &summaries) != nil || len(summaries) == 0 || int64(len(summaries)) > result.MatchCount || (result.Truncated != (int64(len(summaries)) < result.MatchCount)) {
			return spanCommandResult{}, errors.New("invalid ambiguous result")
		}
	case "found":
		if result.TraceID == nil || len(result.Span) == 0 || len(result.Logs) == 0 {
			return spanCommandResult{}, errors.New("incomplete found result")
		}
	default:
		return spanCommandResult{}, fmt.Errorf("unknown status %q", result.Status)
	}
	return result, nil
}

func formatSpanCommandResult(result spanCommandResult) (string, error) {
	switch result.Status {
	case "notFound":
		if result.TraceID == nil {
			return fmt.Sprintf("Span %s was not found.\n", result.SpanID), nil
		}
		return fmt.Sprintf("Span %s was not found in trace %s.\n", result.SpanID, *result.TraceID), nil
	case "ambiguous":
		var summaries []json.RawMessage
		if err := json.Unmarshal(result.Summaries, &summaries); err != nil {
			return "", err
		}
		rows, err := decodeTelemetryRows(summaries, spanSummaryFields)
		if err != nil {
			return "", err
		}
		columns := traceColumns(spanSummaryFields...)
		table := formatQueryColumns(queryResult{Columns: columns, Rows: rows, Truncated: result.Truncated})
		return fmt.Sprintf("SPAN SUMMARIES (%d matches)\n%s", result.MatchCount, table), nil
	case "found":
		return formatFoundSpan(result)
	default:
		return "", errors.New("unsupported span result")
	}
}

func formatFoundSpan(result spanCommandResult) (string, error) {
	var span spanDetail
	if err := decodeExactJSON(result.Span, &span); err != nil {
		return "", err
	}
	var logs []spanLog
	if err := decodeExactJSON(result.Logs, &logs); err != nil {
		return "", err
	}

	spanTable := detailTable(
		[]string{"traceID", "spanID", "parentSpanID", "service", "name", "traceState", "flags", "resourceSchemaURL", "scopeSchemaURL", "scopeName", "scopeVersion", "resourceDropped", "scopeDropped"},
		[][]any{{span.TraceID, span.SpanID, nullableString(span.ParentSpanID), attributeValue(span.Resource.Attributes, "service.name"), span.Name, span.TraceState, span.Flags,
			span.ResourceSchemaURL, span.ScopeSchemaURL, span.Scope.Name, span.Scope.Version, span.Resource.DroppedAttributesCount, span.Scope.DroppedAttributesCount}},
	)
	timingTable := detailTable(
		[]string{"startTime", "endTime", "durationNs", "kindCode", "kind", "statusCodeValue", "status", "statusMessage", "droppedAttributes", "droppedEvents", "droppedLinks"},
		[][]any{{span.StartTime, span.EndTime, subtractDecimal(span.EndTime, span.StartTime), span.KindCode, span.Kind, span.StatusCodeValue, span.StatusCode, span.StatusMessage,
			span.DroppedAttributesCount, span.DroppedEventsCount, span.DroppedLinksCount}},
	)
	eventRows := make([][]any, len(span.Events))
	for i, event := range span.Events {
		eventRows[i] = []any{event.Timestamp, event.Name, event.DroppedAttributesCount, compactAttributes(event.Attributes)}
	}
	linkRows := make([][]any, len(span.Links))
	for i, link := range span.Links {
		linkRows[i] = []any{nullableString(link.TraceID), nullableString(link.SpanID), link.TraceState, link.Flags, link.DroppedAttributesCount, compactAttributes(link.Attributes)}
	}
	logRows := make([][]any, len(logs))
	for i, log := range logs {
		bodyKind, bodyValue := compactTaggedValue(log.Body)
		logRows[i] = []any{log.ID, log.Timestamp, log.ObservedTimestamp, log.TraceID, nullableString(log.SpanID), log.SeverityText, log.SeverityNumber,
			attributeValue(log.Resource.Attributes, "service.name"), log.EventName, bodyKind, bodyValue, log.Flags, log.DroppedAttributesCount,
			log.ResourceSchemaURL, log.ScopeSchemaURL, log.Scope.Name, log.Scope.Version, log.Resource.DroppedAttributesCount, log.Scope.DroppedAttributesCount,
			compactAttributes(log.Attributes), compactAttributes(log.Resource.Attributes), compactAttributes(log.Scope.Attributes)}
	}

	return "SPAN\n" + spanTable +
		"\nTIMING AND STATUS\n" + timingTable +
		"\nRESOURCE ATTRIBUTES\n" + formatAttributes(span.Resource.Attributes) +
		"\nSPAN ATTRIBUTES\n" + formatAttributes(span.Attributes) +
		fmt.Sprintf("\nEVENTS (%d)\n", len(span.Events)) + detailTable([]string{"timestamp", "name", "droppedAttributes", "attributes"}, eventRows) +
		fmt.Sprintf("\nLINKS (%d)\n", len(span.Links)) + detailTable([]string{"traceID", "spanID", "traceState", "flags", "droppedAttributes", "attributes"}, linkRows) +
		fmt.Sprintf("\nCORRELATED LOGS (%d)\n", len(logs)) + detailTable([]string{"id", "timestamp", "observedTimestamp", "traceID", "spanID", "severityText", "severityNumber", "service", "eventName", "bodyKind", "body", "flags", "droppedAttributes", "resourceSchemaURL", "scopeSchemaURL", "scopeName", "scopeVersion", "resourceDropped", "scopeDropped", "attributes", "resourceAttributes", "scopeAttributes"}, logRows), nil
}

func decodeExactJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func detailTable(columns []string, rows [][]any) string {
	return formatQueryColumns(queryResult{Columns: traceColumns(columns...), Rows: rows})
}

func formatAttributes(attributes []detailAttr) string {
	rows := make([][]any, len(attributes))
	for i, attribute := range attributes {
		kind, value := compactTaggedValue(attribute.Value)
		rows[i] = []any{attribute.Key, kind, value}
	}
	return detailTable([]string{"key", "kind", "value"}, rows)
}

func compactAttributes(attributes []detailAttr) string {
	parts := make([]string, len(attributes))
	for i, attribute := range attributes {
		kind, value := compactTaggedValue(attribute.Value)
		parts[i] = fmt.Sprintf("%s (%s)=%s", attribute.Key, kind, value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func compactTaggedValue(raw json.RawMessage) (string, string) {
	var tagged struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &tagged); err != nil || tagged.Kind == "" {
		return "unknown", string(raw)
	}
	var scalar string
	if tagged.Kind == "string" || tagged.Kind == "int64" || tagged.Kind == "bytes" {
		if json.Unmarshal(tagged.Value, &scalar) == nil {
			return tagged.Kind, scalar
		}
	}
	return tagged.Kind, string(tagged.Value)
}

func attributeValue(attributes []detailAttr, key string) string {
	for _, attribute := range attributes {
		if attribute.Key == key {
			_, value := compactTaggedValue(attribute.Value)
			return value
		}
	}
	return ""
}

func subtractDecimal(left, right string) string {
	a, aOK := new(big.Int).SetString(left, 10)
	b, bOK := new(big.Int).SetString(right, 10)
	if !aOK || !bOK {
		return ""
	}
	return new(big.Int).Sub(a, b).String()
}
