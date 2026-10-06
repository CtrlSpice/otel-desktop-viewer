package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type traceResult struct {
	Trace traceSummary `json:"trace"`
	Spans []traceSpan  `json:"spans"`
	Logs  []traceLog   `json:"logs"`
}

type traceSummary struct {
	TraceID    string `json:"traceID"`
	SpanCount  int64  `json:"spanCount"`
	LogCount   int64  `json:"logCount"`
	StartTime  string `json:"startTime"`
	DurationNs string `json:"durationNs"`
}

type traceSpan struct {
	SpanID        string  `json:"spanID"`
	ParentSpanID  *string `json:"parentSpanID"`
	Service       string  `json:"service"`
	Name          string  `json:"name"`
	StartOffsetNs string  `json:"startOffsetNs"`
	DurationNs    string  `json:"durationNs"`
}

type traceLog struct {
	Timestamp string  `json:"timestamp"`
	SpanID    *string `json:"spanID"`
	Severity  string  `json:"severity"`
	Service   string  `json:"service"`
	EventName string  `json:"eventName"`
	Body      string  `json:"body"`
}

func newTraceCommand(client *http.Client) *cobra.Command {
	var endpoint string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "trace <trace-id>",
		Short: "🧵 Inspect one trace overview",
		Long: "🧵 Inspect compact rows for every stored span and trace-linked log. " +
			"--json mirrors the same compact fields. " +
			"Trace start is the minimum stored span start; trace duration is maximum span end minus minimum span start; " +
			"span start offset is stored span start minus trace start; span duration is stored end minus stored start. These values are exact nanoseconds. " +
			"Log timestamps use the stored timestamp when non-zero, otherwise the stored observed timestamp.",
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
			raw, err := requestViewerRPC(cmd.Context(), client, endpoint, "getTrace", map[string]any{"traceID": traceID})
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
			result, err := decodeTrace(raw)
			if err != nil {
				return fmt.Errorf("decode viewer getTrace result: %w", err)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatTrace(result))
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the compact trace overview as JSON instead of sections")
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

func decodeTrace(raw json.RawMessage) (traceResult, error) {
	var result traceResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return traceResult{}, err
	}
	if result.Spans == nil {
		result.Spans = []traceSpan{}
	}
	if result.Logs == nil {
		result.Logs = []traceLog{}
	}
	return result, nil
}

func formatTrace(result traceResult) string {
	spanRows := make([][]any, len(result.Spans))
	for i, span := range result.Spans {
		spanRows[i] = []any{span.SpanID, nullableString(span.ParentSpanID), span.Service,
			span.Name, span.StartOffsetNs, span.DurationNs}
	}
	logRows := make([][]any, len(result.Logs))
	for i, log := range result.Logs {
		logRows[i] = []any{log.Timestamp, nullableString(log.SpanID), log.Severity,
			log.Service, log.EventName, log.Body}
	}

	traceTable := formatQueryColumns(queryResult{Columns: traceColumns("traceID", "spanCount", "logCount", "startTime", "durationNs"), Rows: [][]any{{
		result.Trace.TraceID, result.Trace.SpanCount, result.Trace.LogCount, result.Trace.StartTime, result.Trace.DurationNs,
	}}})
	spanTable := formatQueryColumns(queryResult{Columns: traceColumns("spanID", "parentSpanID", "service", "name", "startOffsetNs", "durationNs"), Rows: spanRows})
	logTable := formatQueryColumns(queryResult{Columns: traceColumns("timestamp", "spanID", "severity", "service", "eventName", "body"), Rows: logRows})
	return "TRACE\n" + traceTable + "\nSPANS\n" + spanTable + "\nTRACE LOGS\n" + logTable
}

func traceColumns(names ...string) []queryColumn {
	columns := make([]queryColumn, len(names))
	for i, name := range names {
		columns[i] = queryColumn{Name: name}
	}
	return columns
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
