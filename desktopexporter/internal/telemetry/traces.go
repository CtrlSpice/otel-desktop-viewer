package telemetry

import "encoding/json"

type TraceData struct {
	TraceID string     `json:"traceID"`
	Spans   []SpanData `json:"spans"`
}

type TraceSummaries struct {
	TraceSummaries []TraceSummary `json:"traceSummaries"`
}

type TraceSummary struct {
	TraceID   string    `json:"traceID"`
	RootSpan  *RootSpan `json:"rootSpan,omitempty"`
	SpanCount uint32    `json:"spanCount"`
}

type RootSpan struct {
	ServiceName string `json:"serviceName"`
	Name        string `json:"name"`
	StartTime   int64  `json:"-"`
	EndTime     int64  `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for RootSpan
func (rootSpan RootSpan) MarshalJSON() ([]byte, error) {
	type Alias RootSpan // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		StartTime PreciseTimestamp `json:"startTime"`
		EndTime   PreciseTimestamp `json:"endTime"`
	}{
		Alias:     Alias(rootSpan),
		StartTime: NewPreciseTimestamp(rootSpan.StartTime),
		EndTime:   NewPreciseTimestamp(rootSpan.EndTime),
	})
} 