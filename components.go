// This file is maintained by hand; no builder manifest regenerates it.
// TestComponentModuleVersionsMatchGoMod keeps component versions aligned with go.mod.

package main

import (
	desktopexporter "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
	duckdbextension "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/otelcol"
	batchprocessor "go.opentelemetry.io/collector/processor/batchprocessor"
	otlpreceiver "go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

func components() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	// The collector requires an explicit factory for its telemetry providers.
	factories.Telemetry = otelconftelemetry.NewFactory()

	factories.Extensions, err = otelcol.MakeFactoryMap(
		duckdbextension.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ExtensionModules = make(map[component.Type]string, len(factories.Extensions))
	factories.ExtensionModules[duckdbextension.NewFactory().Type()] = "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"

	factories.Receivers, err = otelcol.MakeFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ReceiverModules = make(map[component.Type]string, len(factories.Receivers))
	factories.ReceiverModules[otlpreceiver.NewFactory().Type()] = "go.opentelemetry.io/collector/receiver/otlpreceiver v0.162.0"

	factories.Exporters, err = otelcol.MakeFactoryMap(
		desktopexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ExporterModules = make(map[component.Type]string, len(factories.Exporters))
	factories.ExporterModules[desktopexporter.NewFactory().Type()] = "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"

	factories.Processors, err = otelcol.MakeFactoryMap(
		batchprocessor.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ProcessorModules = make(map[component.Type]string, len(factories.Processors))
	factories.ProcessorModules[batchprocessor.NewFactory().Type()] = "go.opentelemetry.io/collector/processor/batchprocessor v0.162.0"

	factories.Connectors, err = otelcol.MakeFactoryMap[connector.Factory]()
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ConnectorModules = make(map[component.Type]string, len(factories.Connectors))

	return factories, nil
}
