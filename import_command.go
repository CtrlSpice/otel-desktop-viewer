package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newImportCommand(client *http.Client) *cobra.Command {
	var endpoint string
	cmd := &cobra.Command{
		Use: "import <file> [file...]", Short: "📥 Import OTLP JSON files into a running viewer",
		Long: "📥 Import JSON or JSONL files into an existing viewer through its OTLP HTTP receiver. " +
			"Validates each complete file before sending, preserves telemetry bytes, and splits requests at 20 MiB. " +
			"Files and requests are sent sequentially; later files continue after a failure. " +
			"Profiles support is coming soon.",
		Example: "  otel-desktop-viewer import checkout-017.json --endpoint http://localhost:8000\n" +
			"  otel-desktop-viewer import traces.json logs.json metrics.jsonl",
		Args: cobra.MinimumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, files []string) error {
			var rows [][]any
			failed := false
			for _, path := range files {
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				accepted, issues, err := importOTLPFile(cmd.Context(), client, endpoint, path)
				if err != nil {
					if cmd.Context().Err() != nil {
						return cmd.Context().Err()
					}
					issues = append(issues, err.Error())
				}
				result := "Received"
				if accepted == 0 {
					result = "No requests sent"
				}
				if len(issues) > 0 {
					failed = true
					result = "Rejected: " + strings.Join(issues, "; ")
				}
				rows = append(rows, []any{path, accepted, result})
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), detailTable([]string{"FILE", "ACCEPTED REQUESTS", "RESULT"}, rows)); err != nil {
				return err
			}
			if failed {
				return errors.New("one or more files had import issues; accepted requests may already have stored telemetry")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
	return cmd
}

func importOTLPFile(ctx context.Context, client *http.Client, endpoint, path string) (int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, nil, err
	}
	if !info.Mode().IsRegular() {
		return 0, nil, errors.New("import requires a regular JSON or JSONL file")
	}
	plan, err := scanImportFile(ctx, file, info.Size())
	if err != nil {
		return 0, nil, err
	}
	issues := plan.issues
	if len(plan.resources[0])+len(plan.resources[1])+len(plan.resources[2]) == 0 {
		return 0, issues, nil
	}
	raw, err := requestViewerRPC(ctx, client, endpoint, "getImportConfig", nil)
	if err != nil {
		return 0, issues, err
	}
	var config struct {
		Port int `json:"otlpHttpPort"`
	}
	if err := json.Unmarshal(raw, &config); err != nil || config.Port < 1 || config.Port > 65535 {
		return 0, issues, errors.New("viewer returned an invalid OTLP HTTP port")
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return 0, issues, err
	}
	base.Scheme = "http"
	base.Host = net.JoinHostPort(base.Hostname(), strconv.Itoa(config.Port))
	base.User, base.RawQuery, base.Fragment, base.RawPath = nil, "", "", ""
	accepted := 0
	for kind, resources := range plan.resources {
		base.Path = "/v1/" + importSignals[kind]
		report := func(issue string) { issues = append(issues, importSignals[kind]+": "+issue) }
		err := batchImportFile(ctx, file, kind, resources, importRequestBytes, func(body io.Reader, size int64) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), body)
			if err != nil {
				return err
			}
			request.ContentLength = size
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				report(err.Error())
				return nil
			}
			issue, err := importResponseIssue(response)
			if err != nil {
				report(err.Error())
			} else if issue != "" {
				report(issue)
			} else {
				accepted++
			}
			return ctx.Err()
		}, report)
		if err != nil {
			return accepted, issues, err
		}
	}
	return accepted, issues, nil
}

func importResponseIssue(response *http.Response) (string, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	parseErr := json.Unmarshal(body, &fields)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var message string
		if parseErr == nil {
			_ = json.Unmarshal(fields["message"], &message)
		}
		if strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message), nil
		}
		if text := strings.TrimSpace(string(body)); text != "" {
			return text, nil
		}
		return "HTTP " + response.Status, nil
	}
	if parseErr != nil || fields == nil {
		return "", errors.New("invalid OTLP response: expected a JSON object")
	}
	partial, exists := fields["partialSuccess"]
	if !exists {
		return "", nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(partial, &values); err != nil || values == nil {
		return "", errors.New("invalid OTLP partial-success response")
	}
	count := "0"
	for _, key := range []string{"rejectedSpans", "rejectedLogRecords", "rejectedDataPoints"} {
		value, exists := values[key]
		if !exists || isJSONNull(value) {
			continue
		}
		if len(value) > 0 && value[0] == '"' {
			if err := json.Unmarshal(value, &count); err != nil {
				return "", errors.New("invalid OTLP rejected-record count")
			}
		} else {
			// Numeric response counts follow the browser's safe-integer validation.
			var number float64
			if err := json.Unmarshal(value, &number); err != nil || number < 0 || number > 9007199254740991 || number != float64(uint64(number)) {
				return "", errors.New("invalid OTLP rejected-record count")
			}
			count = strconv.FormatUint(uint64(number), 10)
		}
		break
	}
	if count == "" {
		return "", errors.New("invalid OTLP rejected-record count")
	}
	for _, digit := range count {
		if digit < '0' || digit > '9' {
			return "", errors.New("invalid OTLP rejected-record count")
		}
	}
	var message string
	if raw, exists := values["errorMessage"]; exists {
		if isJSONNull(raw) || json.Unmarshal(raw, &message) != nil {
			return "", errors.New("invalid OTLP error message")
		}
	}
	if message != "" {
		return message, nil
	}
	if strings.TrimLeft(count, "0") != "" {
		return fmt.Sprintf("The receiver rejected %s records", count), nil
	}
	return "", nil
}
