package telemetry

import (
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
)

type SampleTelemetry struct {
	Spans   []traces.SpanData
	Logs    []logs.LogData
	Metrics []metrics.MetricData
}

func NewSampleTelemetry() SampleTelemetry {
	sample := SampleTelemetry{}
	sample.Spans = traces.GenerateSampleTraces()
	sample.Logs = logs.GenerateSampleLogs()
	sample.Metrics = metrics.GenerateSampleMetrics()
	return sample
}
