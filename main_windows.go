// go:build windows
// +build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"

	"go.opentelemetry.io/collector/otelcol"
)

func run(buildInfo component.BuildInfo) error {
	if useInteractiveMode, err := checkUseInteractiveMode(); err != nil {
		return err
	} else if useInteractiveMode {
		return runInteractive(buildInfo)
	} else {
		return runService(buildInfo)
	}
}

func checkUseInteractiveMode() (bool, error) {
	// If environment variable NO_WINDOWS_SERVICE is set with any value other
	// than 0, use interactive mode instead of running as a service. This should
	// be set in case running as a service is not possible or desired even
	// though the current session is not detected to be interactive
	if value, present := os.LookupEnv("NO_WINDOWS_SERVICE"); present && value != "0" {
		return true, nil
	}

	isWindowsService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("failed to determine if we are running in an interactive session: %w", err)
	}
	return !isWindowsService, nil
}

func runService(buildInfoparams component.BuildInfo) error {
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
    endpoint: localhost:8000

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

	// do not need to supply service name when startup is invoked through Service Control Manager directly
	if err := svc.Run("", otelcol.NewSvcHandler(collectorSettings)); err != nil {
		return fmt.Errorf("failed to start collector server: %w", err)
	}

	return nil
}
