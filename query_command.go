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
	"unicode"

	"github.com/spf13/cobra"
)

const queryDefaultLimit uint64 = 25

var queryHTTPClient = http.DefaultClient

type queryColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type queryResult struct {
	Columns   []queryColumn `json:"columns"`
	Rows      [][]any       `json:"rows"`
	Truncated bool          `json:"truncated"`
}

type queryRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type queryRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *queryRPCError  `json:"error"`
}

type queryRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newQueryCommand() *cobra.Command {
	var endpoint string
	var limit uint64
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "🔎 Run read-only SQL against the running viewer",
		Long: "🔎 Run one read-only DuckDB query against the existing viewer process. " +
			"Results use aligned columns by default; --json emits the JSON result.",
		Example: "  otel-desktop-viewer query 'SHOW TABLES'\n" +
			"  otel-desktop-viewer query 'DESCRIBE spans'\n" +
			"  otel-desktop-viewer query 'SELECT service_name, count(*) FROM spans GROUP BY service_name' --limit 50\n" +
			"  otel-desktop-viewer query 'SELECT count(*) FROM logs' --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			raw, result, err := requestQuery(cmd.Context(), queryHTTPClient, endpoint, args[0], limit)
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
			_, err = io.WriteString(cmd.OutOrStdout(), formatQueryColumns(result))
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().Uint64Var(&limit, "limit", queryDefaultLimit, "Maximum rows to return")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit the JSON result instead of columns")
	return cmd
}

func requestQuery(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	statement string,
	limit uint64,
) (json.RawMessage, queryResult, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, queryResult{}, fmt.Errorf("invalid viewer endpoint %q", endpoint)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/rpc"

	body, err := json.Marshal(queryRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "query",
		Params:  map[string]any{"sql": statement, "limit": limit},
	})
	if err != nil {
		return nil, queryResult{}, fmt.Errorf("encode query request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, queryResult{}, fmt.Errorf("create viewer request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, queryResult{}, fmt.Errorf("contact viewer at %s: %w", base.String(), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, queryResult{}, fmt.Errorf("viewer returned HTTP %s", response.Status)
	}

	var rpcResponse queryRPCResponse
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&rpcResponse); err != nil {
		return nil, queryResult{}, fmt.Errorf("decode viewer response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, queryResult{}, fmt.Errorf("decode viewer response: additional JSON value")
		}
		return nil, queryResult{}, fmt.Errorf("decode viewer response: trailing data: %w", err)
	}
	if rpcResponse.Error != nil {
		return nil, queryResult{}, fmt.Errorf("viewer query error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}
	if len(rpcResponse.Result) == 0 {
		return nil, queryResult{}, fmt.Errorf("decode viewer response: missing result")
	}

	var result queryResult
	resultDecoder := json.NewDecoder(bytes.NewReader(rpcResponse.Result))
	resultDecoder.UseNumber()
	if err := resultDecoder.Decode(&result); err != nil {
		return nil, queryResult{}, fmt.Errorf("decode query result: %w", err)
	}
	return rpcResponse.Result, result, nil
}

func formatQueryColumns(result queryResult) string {
	if len(result.Columns) == 0 {
		return ""
	}

	widths := make([]int, len(result.Columns))
	headers := make([]string, len(result.Columns))
	for i, column := range result.Columns {
		headers[i] = escapeQueryDisplay(column.Name)
		widths[i] = queryDisplayWidth(headers[i])
	}

	rows := make([][]string, len(result.Rows))
	for rowIndex, row := range result.Rows {
		rows[rowIndex] = make([]string, len(result.Columns))
		for columnIndex := range result.Columns {
			value := "NULL"
			if columnIndex < len(row) && row[columnIndex] != nil {
				value = formatQueryValue(row[columnIndex])
			}
			value = escapeQueryDisplay(value)
			rows[rowIndex][columnIndex] = value
			widths[columnIndex] = max(widths[columnIndex], queryDisplayWidth(value))
		}
	}

	var output strings.Builder
	writeLine := func(values []string) {
		for i, value := range values {
			if i > 0 {
				output.WriteString("  ")
			}
			output.WriteString(value)
			output.WriteString(strings.Repeat(" ", widths[i]-queryDisplayWidth(value)))
		}
		output.WriteByte('\n')
	}
	writeLine(headers)
	underlines := make([]string, len(widths))
	for i, width := range widths {
		underlines[i] = strings.Repeat("-", width)
	}
	writeLine(underlines)
	for _, row := range rows {
		writeLine(row)
	}
	if result.Truncated {
		fmt.Fprintf(&output, "[%d rows shown; more rows available; use --limit to return more]\n", len(result.Rows))
	}
	return output.String()
}

func formatQueryValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case json.Number:
		return value.String()
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

func escapeQueryDisplay(value string) string {
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

func queryDisplayWidth(value string) int {
	width := 0
	for _, r := range value {
		if !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Me, r) {
			width++
		}
	}
	return width
}
