package main

import (
	"log"
	"strconv"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

func main() {
	info := component.BuildInfo{
		Command:     "otel-desktop-viewer",
		Description: "Basic OTel with Custom Desktop Exporter",
		Version:     "0.1.1",
	}

	if err := run(info); err != nil {
		log.Fatal(err)
	}
}

func runInteractive(buildInfo component.BuildInfo) error {
	cmd := newCommand(buildInfo)
	if err := cmd.Execute(); err != nil {
		log.Fatalf("collector server run finished with error: %v", err)
	}

	return nil
}

func newCommand(info component.BuildInfo) *cobra.Command {
	var httpPortFlag, grpcPortFlag, browserPortFlag int

	rootCmd := &cobra.Command{
		Use:          info.Command,
		Version:      info.Version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configContents := `yaml:
receivers:
  otlp:
    protocols:
      http:
        endpoint: localhost:` + strconv.Itoa(httpPortFlag) + `
      grpc:
        endpoint: localhost:` + strconv.Itoa(grpcPortFlag) + `

processors:

exporters:
  desktop:
    endpoint: localhost:` + strconv.Itoa(browserPortFlag) + `

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: []
      exporters: [desktop]
    metrics:
      receivers: [otlp]
      processors: []
      exporters: [desktop]
    logs:
      receivers: [otlp]
      processors: []
      exporters: [desktop]
`

			factories, err := components()
			if err != nil {
				log.Fatalf("failed to build components: %v", err)
			}

			provider := yamlprovider.New()
			configProviderSettings := otelcol.ConfigProviderSettings{
				ResolverSettings: confmap.ResolverSettings{
					URIs:      []string{configContents},
					Providers: map[string]confmap.Provider{provider.Scheme(): provider},
				},
			}

			configProvider, err := otelcol.NewConfigProvider(configProviderSettings)
			if err != nil {
				log.Fatal(err)
			}

			collectorSettings := otelcol.CollectorSettings{
				BuildInfo:      info,
				Factories:      factories,
				ConfigProvider: configProvider,
			}

			col, err := otelcol.NewCollector(collectorSettings)
			if err != nil {
				return err
			}
			return col.Run(cmd.Context())
		},
	}

	rootCmd.Flags().IntVar(&httpPortFlag, "http", 4318, "The port number on which we listen for OTLP http payloads")
	rootCmd.Flags().IntVar(&grpcPortFlag, "grpc", 4317, "The port number on which we listen for OTLP grpc payloads")
	rootCmd.Flags().IntVar(&browserPortFlag, "browser", 8000, "The port number where we expose our data")
	return rootCmd
}
