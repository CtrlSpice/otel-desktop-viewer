package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	telemetryDefaultLimit int64 = 25
	telemetryDefaultSince       = time.Hour
)

type telemetrySearchOptions struct {
	Endpoint string
	Service  string
	Since    time.Duration
	Start    string
	End      string
	Limit    int64
}

type telemetrySearchQuery struct {
	Service   string
	StartTime *string
	EndTime   *string
	Limit     int64
}

type telemetrySearchResult struct {
	Summaries []json.RawMessage
	Rows      [][]any
	Truncated bool
}

func addTelemetrySearchFlags(cmd *cobra.Command, options *telemetrySearchOptions, jsonOutput *bool) {
	cmd.Flags().StringVar(&options.Endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	cmd.Flags().StringVar(&options.Service, "service", "", "Only summaries for this service")
	cmd.Flags().DurationVar(&options.Since, "since", telemetryDefaultSince, "Relative lookback window (for example 30m, 6h, or 24h)")
	cmd.Flags().StringVar(&options.Start, "start", "", "Inclusive RFC3339 start time (nanoseconds supported)")
	cmd.Flags().StringVar(&options.End, "end", "", "Inclusive RFC3339 end time (nanoseconds supported)")
	cmd.Flags().Int64Var(&options.Limit, "limit", telemetryDefaultLimit, "Maximum summaries to return")
	cmd.Flags().BoolVar(jsonOutput, "json", false, "Emit the summary response as JSON instead of columns")
}

func resolveTelemetrySearch(options telemetrySearchOptions, sinceChanged bool, now time.Time) (telemetrySearchQuery, error) {
	if options.Limit < 1 {
		return telemetrySearchQuery{}, fmt.Errorf("--limit must be greater than zero")
	}
	if options.Limit == math.MaxInt64 {
		return telemetrySearchQuery{}, fmt.Errorf("--limit is too large")
	}
	if sinceChanged && (options.Start != "" || options.End != "") {
		return telemetrySearchQuery{}, fmt.Errorf("--since cannot be combined with --start or --end")
	}
	if options.Since <= 0 {
		return telemetrySearchQuery{}, fmt.Errorf("--since must be greater than zero")
	}

	query := telemetrySearchQuery{Service: options.Service, Limit: options.Limit}
	if options.Start == "" && options.End == "" {
		end, err := exactTelemetryUnixNano(now, "current time")
		if err != nil {
			return telemetrySearchQuery{}, err
		}
		start, err := exactTelemetryUnixNano(now.Add(-options.Since), "--since start")
		if err != nil {
			return telemetrySearchQuery{}, err
		}
		query.StartTime = telemetryDecimalString(start)
		query.EndTime = telemetryDecimalString(end)
		return query, nil
	}

	var start, end *uint64
	var err error
	if options.Start != "" {
		start, err = parseTelemetryTime(options.Start, "--start")
		if err != nil {
			return telemetrySearchQuery{}, err
		}
		query.StartTime = telemetryDecimalString(*start)
	}
	if options.End != "" {
		end, err = parseTelemetryTime(options.End, "--end")
		if err != nil {
			return telemetrySearchQuery{}, err
		}
		query.EndTime = telemetryDecimalString(*end)
	}
	if start != nil && end != nil && *start > *end {
		return telemetrySearchQuery{}, fmt.Errorf("--start must not be after --end")
	}
	return query, nil
}

func parseTelemetryTime(value, flag string) (*uint64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: use RFC3339, for example 2026-10-02T09:30:00.123456789Z", flag, value)
	}
	nano, err := exactTelemetryUnixNano(parsed, flag)
	if err != nil {
		return nil, err
	}
	return &nano, nil
}

func exactTelemetryUnixNano(value time.Time, source string) (uint64, error) {
	seconds := value.Unix()
	if seconds < 0 || uint64(seconds) > (math.MaxUint64-uint64(value.Nanosecond()))/uint64(time.Second) {
		return 0, fmt.Errorf("%s is outside the supported nanosecond timestamp range", source)
	}
	return uint64(seconds)*uint64(time.Second) + uint64(value.Nanosecond()), nil
}

func telemetryDecimalString(value uint64) *string {
	formatted := strconv.FormatUint(value, 10)
	return &formatted
}

func telemetryServiceQuery(method, service string) any {
	if service == "" {
		return nil
	}
	field := map[string]any{
		"name":        "serviceName",
		"searchScope": "field",
		"type":        "string",
	}
	if method == "searchMetricSummaries" {
		field = map[string]any{
			"name":           "service.name",
			"searchScope":    "attribute",
			"attributeScope": "resource",
			"type":           "string",
		}
	}
	return map[string]any{
		"id":   "cli-service",
		"type": "condition",
		"query": map[string]any{
			"field":         field,
			"fieldOperator": "=",
			"value":         service,
		},
	}
}

func requestTelemetrySearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	method string,
	query telemetrySearchQuery,
	fields []string,
) (telemetrySearchResult, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return telemetrySearchResult{}, fmt.Errorf("invalid viewer endpoint %q", endpoint)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/rpc"
	body, err := json.Marshal(queryRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params: map[string]any{
			"startTime": query.StartTime,
			"endTime":   query.EndTime,
			"query":     telemetryServiceQuery(method, query.Service),
			"limit":     query.Limit + 1,
		},
	})
	if err != nil {
		return telemetrySearchResult{}, fmt.Errorf("encode %s request: %w", method, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return telemetrySearchResult{}, fmt.Errorf("create viewer request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return telemetrySearchResult{}, fmt.Errorf("contact viewer at %s: %w", base.String(), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return telemetrySearchResult{}, fmt.Errorf("viewer returned HTTP %s", response.Status)
	}

	var rpcResponse queryRPCResponse
	if err := json.NewDecoder(response.Body).Decode(&rpcResponse); err != nil {
		return telemetrySearchResult{}, fmt.Errorf("decode viewer response: %w", err)
	}
	if rpcResponse.Error != nil {
		return telemetrySearchResult{}, fmt.Errorf("viewer %s error %d: %s", method, rpcResponse.Error.Code, rpcResponse.Error.Message)
	}
	if len(rpcResponse.Result) == 0 {
		return telemetrySearchResult{}, fmt.Errorf("decode viewer response: missing result")
	}

	var summaries []json.RawMessage
	if err := json.Unmarshal(rpcResponse.Result, &summaries); err != nil {
		return telemetrySearchResult{}, fmt.Errorf("decode viewer %s result: %w", method, err)
	}
	truncated := int64(len(summaries)) > query.Limit
	if truncated {
		summaries = summaries[:query.Limit]
	}
	if summaries == nil {
		summaries = []json.RawMessage{}
	}
	rows, err := decodeTelemetryRows(summaries, fields)
	if err != nil {
		return telemetrySearchResult{}, fmt.Errorf("decode viewer %s result: %w", method, err)
	}
	return telemetrySearchResult{Summaries: summaries, Rows: rows, Truncated: truncated}, nil
}

func decodeTelemetryRows(summaries []json.RawMessage, fields []string) ([][]any, error) {
	rows := make([][]any, len(summaries))
	for rowIndex, raw := range summaries {
		var summary map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&summary); err != nil {
			return nil, err
		}
		row := make([]any, len(fields))
		for columnIndex, field := range fields {
			value, ok := summary[field]
			if !ok {
				return nil, fmt.Errorf("summary %d is missing field %q", rowIndex, field)
			}
			row[columnIndex] = value
		}
		rows[rowIndex] = row
	}
	return rows, nil
}

func writeTelemetrySearchResult(writer io.Writer, result telemetrySearchResult, fields []string, jsonOutput bool) error {
	if jsonOutput {
		if _, err := io.WriteString(writer, "["); err != nil {
			return err
		}
		for i, summary := range result.Summaries {
			if i > 0 {
				if _, err := io.WriteString(writer, ","); err != nil {
					return err
				}
			}
			if _, err := writer.Write(summary); err != nil {
				return err
			}
		}
		_, err := io.WriteString(writer, "]\n")
		return err
	}
	columns := make([]queryColumn, len(fields))
	for i, field := range fields {
		columns[i] = queryColumn{Name: field}
	}
	_, err := io.WriteString(writer, formatQueryColumns(queryResult{
		Columns: columns, Rows: result.Rows, Truncated: result.Truncated,
	}))
	return err
}
