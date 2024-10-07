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
	HasRootSpan bool `json:"hasRootSpan"`

	RootServiceName string    `json:"rootServiceName"`
	RootName        string    `json:"rootName"`
	RootStartTime   time.Time `json:"rootStartTime"`
	RootEndTime     time.Time `json:"rootEndTime"`

	SpanCount uint32 `json:"spanCount"`
	TraceID   string `json:"traceID"`
}
