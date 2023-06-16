package telemetry

import (
	"time"
)

func (t *TelemetryData) GetSummary() TelemetrySummary {

	switch t.Type {
	case "trace":
		traceSummary := t.Trace.GetTraceSummary()
		rootSpan, err := t.Trace.getRootSpan()

		if err == WarningMissingRootSpan {
			return TelemetrySummary{
				HasRootSpan:     false,
				RootServiceName: "",
				RootName:        "",
				RootStartTime:   time.Time{},
				RootEndTime:     time.Time{},
				SpanCount:       uint32(len(t.Trace.Spans)),
				ID:              traceSummary.TraceID,
				Type:            t.Type,
				ServiceName:     "",
			}
		}

		return TelemetrySummary{
			HasRootSpan:     true,
			RootServiceName: rootSpan.GetServiceName(),
			RootName:        rootSpan.Name,
			RootStartTime:   rootSpan.StartTime,
			RootEndTime:     rootSpan.EndTime,
			SpanCount:       uint32(len(t.Trace.Spans)),
			ID:              traceSummary.TraceID,
			Type:            t.Type,
		}
	}

	return TelemetrySummary{
		HasRootSpan:     false,
		RootServiceName: "", //maybe we can omit these?
		RootName:        "",
		RootStartTime:   time.Time{},
		RootEndTime:     time.Time{},
		ServiceName:     "",
		SpanCount:       0,
		ID:              t.ID,
		Type:            t.Type,
	}
}
