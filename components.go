package main

import (
	"go.opentelemetry.io/collector/component"
	//loggingexporter "go.opentelemetry.io/collector/exporter/loggingexporter"
	desktopexporter "github.com/CtrlSpice/desktop-collector/desktop-exporter"
	batchprocessor "go.opentelemetry.io/collector/processor/batchprocessor"
	otlpreceiver "go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func components() (component.Factories, error) {
	var err error
	factories := component.Factories{}

	factories.Extensions, err = component.MakeExtensionFactoryMap()
	if err != nil {
		return component.Factories{}, err
	}

	factories.Receivers, err = component.MakeReceiverFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	factories.Exporters, err = component.MakeExporterFactoryMap(
		desktopexporter.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	factories.Processors, err = component.MakeProcessorFactoryMap(
		batchprocessor.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	return factories, nil
}
