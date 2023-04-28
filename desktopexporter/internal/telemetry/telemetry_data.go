package telemetry

func (t *TelemetryData) GetSummary() TelemetrySummary {
	switch t.Type {
	case "trace":
		traceSummary := t.Trace.GetTraceSummary()
		return TelemetrySummary{
			Type:        t.Type,
			ID:          traceSummary.TraceID,
			ServiceName: traceSummary.RootServiceName,
		}
	}
	return TelemetrySummary{
		Type: t.Type,
		ID:   t.ID,
	}
}
