// Program otel-desktop-viewer is an OpenTelemetry Collector binary.

package main

import (
	"log"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	envprovider "go.opentelemetry.io/collector/confmap/provider/envprovider"
	yamlprovider "go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

var version = "dev" // overridden by ldflags in release builds

func buildVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	modVersion := info.Main.Version
	if modVersion == "" || modVersion == "(devel)" {
		return version
	}
	return strings.TrimPrefix(modVersion, "v")
}

func main() {
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Command:     "otel-desktop-viewer",
			Description: "Collector distribution that allows developers to visualize their OTel data locally",
			Version:     buildVersion(),
		},
		Factories: components,
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				ProviderFactories: []confmap.ProviderFactory{
					envprovider.NewFactory(),
					yamlprovider.NewFactory(),
				},
			},
		},
	}

	if err := run(set); err != nil {
		log.Fatal(err)
	}
}
