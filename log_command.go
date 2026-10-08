package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newLogCommand(client *http.Client) *cobra.Command {
	var endpoint string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "log <log-ref>",
		Short: "🪵 Inspect one complete log",
		Long: "🪵 Inspect one retained log, including its complete typed body, attributes, resource and scope. " +
			"Use logRef from logs --json: it is a viewer-generated reference valid within that database. " +
			"traceID and spanID are received OTel correlation IDs. --json preserves the detail response, including exact values and nulls.",
		Example: "  otel-desktop-viewer log 018f0000-0000-7000-8000-000000000001\n" +
			"  otel-desktop-viewer log 018f0000-0000-7000-8000-000000000001 --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			ref, err := normalizeDetailRef(args[0], "log")
			if err != nil {
				return err
			}
			raw, err := requestViewerRPC(cmd.Context(), client, endpoint, "getLog", map[string]any{"logRef": ref})
			if err != nil {
				return err
			}
			if jsonOutput {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return err
			}
			formatted, err := formatLogDetail(raw)
			if err != nil {
				return fmt.Errorf("decode viewer getLog result: %w", err)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatted)
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the complete log response as JSON instead of sections")
	return cmd
}

// References use the same UUID/32-character hexadecimal forms as the export CLI.
func normalizeDetailRef(value, kind string) (string, error) {
	text, err := normalizeTraceID(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s reference: expected 32-char hex or a dashed UUID", kind)
	}
	ref, err := uuid.Parse(text)
	if err != nil {
		return "", err
	}
	return ref.String(), nil
}

func formatLogDetail(raw json.RawMessage) (string, error) {
	var log struct {
		LogRef                 string          `json:"logRef"`
		Timestamp              string          `json:"timestamp"`
		ObservedTimestamp      string          `json:"observedTimestamp"`
		TraceID                *string         `json:"traceID"`
		SpanID                 *string         `json:"spanID"`
		SeverityText           string          `json:"severityText"`
		SeverityNumber         json.Number     `json:"severityNumber"`
		Body                   json.RawMessage `json:"body"`
		Flags                  json.Number     `json:"flags"`
		EventName              string          `json:"eventName"`
		DroppedAttributesCount json.Number     `json:"droppedAttributesCount"`
		ResourceSchemaURL      string          `json:"resourceSchemaURL"`
		ScopeSchemaURL         string          `json:"scopeSchemaURL"`
		Resource               detailResource  `json:"resource"`
		Scope                  detailScope     `json:"scope"`
		Attributes             []detailAttr    `json:"attributes"`
	}
	if err := decodeExactJSON(raw, &log); err != nil {
		return "", err
	}
	if log.LogRef == "" || len(log.Body) == 0 {
		return "", fmt.Errorf("missing logRef or body")
	}
	bodyKind, body := compactTaggedValue(log.Body)
	return "LOG\n" + detailTable(
		[]string{"logRef", "timestamp", "observedTimestamp", "traceID", "spanID", "severityText", "severityNumber", "flags", "eventName", "droppedAttributesCount"},
		[][]any{{log.LogRef, log.Timestamp, log.ObservedTimestamp, nullableString(log.TraceID), nullableString(log.SpanID), log.SeverityText, log.SeverityNumber, log.Flags, log.EventName, log.DroppedAttributesCount}},
	) + "\nBODY\n" + detailTable([]string{"kind", "value"}, [][]any{{bodyKind, body}}) +
		"\nRESOURCE\n" + detailTable([]string{"schemaURL", "droppedAttributesCount"}, [][]any{{log.ResourceSchemaURL, log.Resource.DroppedAttributesCount}}) +
		"\nRESOURCE ATTRIBUTES\n" + formatAttributes(log.Resource.Attributes) +
		"\nSCOPE\n" + detailTable([]string{"name", "version", "schemaURL", "droppedAttributesCount"}, [][]any{{log.Scope.Name, log.Scope.Version, log.ScopeSchemaURL, log.Scope.DroppedAttributesCount}}) +
		"\nSCOPE ATTRIBUTES\n" + formatAttributes(log.Scope.Attributes) +
		"\nLOG ATTRIBUTES\n" + formatAttributes(log.Attributes), nil
}
