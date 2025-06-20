package traces

import (
	"encoding/json"
	"strconv"
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
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
	}{
		Alias:     Alias(rootSpan),
		StartTime: strconv.FormatInt(rootSpan.StartTime, 10),
		EndTime:   strconv.FormatInt(rootSpan.EndTime, 10),
	})
}
