package telemetry

import (
	"time"
)

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
	ServiceName string    `json:"serviceName"`
	Name        string    `json:"name"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
}
