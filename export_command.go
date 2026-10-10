package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newExportCommand(client *http.Client) *cobra.Command {
	cmd := &cobra.Command{
		SilenceUsage:  true,
		SilenceErrors: true,
		Use:           "export", Short: "📤 Export stored telemetry as OTLP JSON",
		Long: "📤 Export one complete retained trace, log record, or Metric from a running viewer. " +
			"Exact JSON bytes are written to stdout for redirection to a file.",
	}
	for _, signal := range []string{"trace", "log", "metric"} {
		var endpoint string
		child := &cobra.Command{
			SilenceUsage:  true,
			SilenceErrors: true,
			Use:           signal + " <id>", Short: "📤 Export one stored " + signal,
			Long: "📤 Export one complete stored " + signal + " as OTLP JSON. Includes resource/scope context and all retained members, " +
				"regardless of the current search or time window. " +
				"Writes exact response bytes without a trailing newline; redirect stdout to save a file.",
			Example: "  otel-desktop-viewer export " + signal + " <id> > " + signal + ".json",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := normalizeTraceID(args[0])
				if err != nil {
					return fmt.Errorf("invalid %s identifier: expected 32-char hex or a dashed UUID", signal)
				}
				if signal != "trace" {
					parsed, err := uuid.Parse(id)
					if err != nil {
						return err
					}
					id = parsed.String()
				}
				base, err := url.Parse(endpoint)
				if err != nil || base.Scheme == "" || base.Host == "" {
					return fmt.Errorf("invalid viewer endpoint %q", endpoint)
				}
				base.Path = strings.TrimRight(base.Path, "/") + "/export/" + signal + "s/" + id
				base.RawPath = ""
				base.RawQuery = ""
				request, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, base.String(), nil)
				if err != nil {
					return err
				}
				contentType := "application/json"
				request.Header.Set("Accept", contentType)
				response, err := client.Do(request)
				if err != nil {
					return fmt.Errorf("contact viewer: %w", err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("viewer export returned HTTP %s", response.Status)
				}
				mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
				if err != nil || mediaType != contentType {
					return fmt.Errorf("viewer export returned unexpected content type %q", response.Header.Get("Content-Type"))
				}
				_, err = io.Copy(cmd.OutOrStdout(), response.Body)
				return err
			},
		}
		child.Flags().StringVar(&endpoint, "endpoint", "http://localhost:8000", "Running viewer HTTP endpoint")
		cmd.AddCommand(child)
	}
	return cmd
}
