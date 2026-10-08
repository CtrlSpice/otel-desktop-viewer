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
	var document map[string]any
	if err := decodeExactJSON(raw, &document); err != nil {
		return "", err
	}
	if err := validateLogDetail(document); err != nil {
		return "", err
	}
	resource := document["resource"].(map[string]any)
	scope := document["scope"].(map[string]any)
	body := detailValueDocument(document["body"])
	fields := []string{"logRef", "timestamp", "observedTimestamp", "traceID", "spanID", "severityText", "severityNumber", "flags", "eventName", "droppedAttributesCount"}
	return "LOG\n" + detailTable(fields, [][]any{detailFieldValues(document, fields)}) +
		"\nBODY\n" + detailTable([]string{"kind", "value"}, [][]any{{body["kind"], formatQueryValue(body["value"])}}) +
		"\nRESOURCE\n" + detailTable([]string{"schemaURL", "droppedAttributesCount"}, [][]any{{document["resourceSchemaURL"], resource["droppedAttributesCount"]}}) +
		"\nRESOURCE ATTRIBUTES\n" + formatDetailAttributes(resource["attributes"].([]any)) +
		"\nSCOPE\n" + detailTable([]string{"name", "version", "schemaURL", "droppedAttributesCount"}, [][]any{{scope["name"], scope["version"], document["scopeSchemaURL"], scope["droppedAttributesCount"]}}) +
		"\nSCOPE ATTRIBUTES\n" + formatDetailAttributes(scope["attributes"].([]any)) +
		"\nLOG ATTRIBUTES\n" + formatDetailAttributes(document["attributes"].([]any)), nil
}
