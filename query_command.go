package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/uniseg"
	"github.com/spf13/cobra"
)

const (
	queryDefaultLimit = 25
	queryMaxLimit     = 1000
	queryMaxBytes     = 1 << 20
)

var queryHTTPClient = &http.Client{Timeout: 5 * time.Second}

type queryColumn struct {
	Name       string `json:"name"`
	DuckDBType string `json:"duckdbType"`
	Encoding   string `json:"encoding"`
}

type queryResult struct {
	Columns   []queryColumn `json:"columns"`
	Rows      [][]any       `json:"rows"`
	Limit     int           `json:"limit"`
	RowCount  int           `json:"rowCount"`
	Truncated bool          `json:"truncated"`
}

func newQueryCommand() *cobra.Command {
	var endpoint string
	var limit int
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "🔎 Run read-only SQL against the running viewer",
		Long: "🔎 Run one read-only DuckDB query against the existing viewer process. " +
			"Results use aligned columns by default; --json preserves exact machine-readable values.",
		Example: "  otel-desktop-viewer query 'SHOW TABLES'\n" +
			"  otel-desktop-viewer query 'DESCRIBE spans'\n" +
			"  otel-desktop-viewer query 'SELECT trace_id, name FROM spans' --limit 50 --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			result, err := requestQuery(cmd.Context(), queryHTTPClient, endpoint, args[0], limit)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(result)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatColumns(result))
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().IntVar(&limit, "limit", queryDefaultLimit, "Maximum rows to return (0-1000)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit exact machine-readable JSON instead of columns")
	return cmd
}

type rpcQueryRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type rpcQueryResponse struct {
	Result queryResult `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requestQuery(ctx context.Context, client *http.Client, endpoint, statement string, limit int) (queryResult, error) {
	if limit < 0 || limit > queryMaxLimit {
		return queryResult{}, fmt.Errorf("limit must be between 0 and %d", queryMaxLimit)
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return queryResult{}, fmt.Errorf("invalid viewer endpoint %q", endpoint)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/rpc"
	body, err := json.Marshal(rpcQueryRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "query",
		Params:  map[string]any{"sql": statement, "limit": limit},
	})
	if err != nil {
		return queryResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return queryResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return queryResult{}, fmt.Errorf("contact viewer at %s: %w", base.String(), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return queryResult{}, fmt.Errorf("viewer returned HTTP %s", response.Status)
	}
	var decoded rpcQueryResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, queryMaxBytes+(64<<10)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return queryResult{}, fmt.Errorf("decode viewer response: %w", err)
	}
	if decoded.Error != nil {
		return queryResult{}, fmt.Errorf("viewer query error %d: %s", decoded.Error.Code, decoded.Error.Message)
	}
	return decoded.Result, nil
}

func formatColumns(result queryResult) string {
	if len(result.Columns) == 0 {
		return ""
	}
	widths := make([]int, len(result.Columns))
	headers := make([]string, len(result.Columns))
	for i, column := range result.Columns {
		headers[i] = escapeDisplay(column.Name)
		widths[i] = uniseg.StringWidth(headers[i])
	}
	displayRows := make([][]string, len(result.Rows))
	for rowIndex, row := range result.Rows {
		displayRows[rowIndex] = make([]string, len(result.Columns))
		for columnIndex := range result.Columns {
			value := "NULL"
			if columnIndex < len(row) && row[columnIndex] != nil {
				value = displayValue(row[columnIndex])
			}
			value = escapeDisplay(value)
			displayRows[rowIndex][columnIndex] = value
			widths[columnIndex] = max(widths[columnIndex], uniseg.StringWidth(value))
		}
	}

	var output strings.Builder
	writeLine := func(values []string) {
		for i, value := range values {
			if i > 0 {
				output.WriteString("  ")
			}
			output.WriteString(value)
			output.WriteString(strings.Repeat(" ", widths[i]-uniseg.StringWidth(value)))
		}
		output.WriteByte('\n')
	}
	writeLine(headers)
	underlines := make([]string, len(widths))
	for i, width := range widths {
		underlines[i] = strings.Repeat("-", width)
	}
	writeLine(underlines)
	for _, row := range displayRows {
		writeLine(row)
	}
	if result.Truncated {
		fmt.Fprintf(&output, "[%d rows shown; more rows available, use --limit up to %d]\n", result.RowCount, queryMaxLimit)
	}
	return output.String()
}

func displayValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

func escapeDisplay(value string) string {
	var escaped strings.Builder
	for _, r := range value {
		switch r {
		case '\n':
			escaped.WriteString(`\n`)
		case '\r':
			escaped.WriteString(`\r`)
		case '\t':
			escaped.WriteString(`\t`)
		case '\x1b':
			escaped.WriteString(`\u001b`)
		default:
			if unicode.IsControl(r) {
				fmt.Fprintf(&escaped, `\u%04x`, r)
			} else {
				escaped.WriteRune(r)
			}
		}
	}
	return escaped.String()
}
