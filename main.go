package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

func main() {
	// We want this to be as easy to use as possible
	// so we don't need the user to include a config
	configContents := `yaml:
receivers:
  otlp:
    protocols:
      http:
        endpoint: localhost:4318
      grpc:
        endpoint: localhost:4317

processors:

exporters:
  desktop:

service:
  pipelines:
    traces:
      receivers:
        - otlp
      processors: []
      exporters:
        - desktop`

	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	info := component.BuildInfo{
		Command:     "otel-desktop-viewer",
		Description: "Basic OTel with Custom Desktop Exporter",
		Version:     "0.0.2",
	}

	provider := yamlprovider.New()
	set := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:      []string{configContents},
			Providers: map[string]confmap.Provider{provider.Scheme(): provider},
		},
	}

	configProvider, err := otelcol.NewConfigProvider(set)
	if err != nil {
		log.Fatal(err)
	}

	settings := otelcol.CollectorSettings{
		BuildInfo: info,
		Factories: factories,
		ConfigProvider: configProvider,
	}

	if err := run(settings); err != nil {
		log.Fatal(err)
	}
}

func runInteractive(params otelcol.CollectorSettings) error {
	cmd := otelcol.NewCommand(params)
	if err := cmd.Execute(); err != nil {
		log.Fatalf("collector server run finished with error: %v", err)
	}

	return nil
}
