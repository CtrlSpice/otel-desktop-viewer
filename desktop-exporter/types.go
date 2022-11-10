package desktopexporter

import (
	"errors"
	"time"
)

var ErrEmptySpanSlice = errors.New("slice of spans associated with this traceID must not be empty")
var ErrTraceIDNotFound = errors.New("traceID not found")
var ErrTraceIDMismatch = errors.New("traceID mismatch between TraceStore.traceMap and TraceStore.traceQueue")

type TraceSummaries struct {
	Summaries []TraceSummary `json:"traceSummaries"`
}

type TraceSummary struct {
	TraceID    string `json:"traceID"`
	SpanCount  uint32 `json:"spanCount"`
	DurationMS int64  `json:"durationMS"`
}

type TraceData struct {
	Spans []SpanData `json:"spans"`
}

type ResourceData struct {
	Attributes             map[string]interface{} `json:"attributes"`
	DroppedAttributesCount uint32                 `json:"droppedAttributesCount"`
}

type ScopeData struct {
	Name                   string                 `json:"name"`
	Version                string                 `json:"version"`
	Attributes             map[string]interface{} `json:"attributes"`
	DroppedAttributesCount uint32                 `json:"droppedAttributesCount"`
}

type SpanData struct {
	TraceID      string `json:"traceID"`
	TraceState   string `json:"traceState"`
	SpanID       string `json:"spanID"`
	ParentSpanID string `json:"parentSpanID"`

	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`

	Attributes map[string]interface{} `json:"attributes"`
	Events     []EventData            `json:"events"`
	Links      []LinkData             `json:"links"`
	Resource   *ResourceData          `json:"resource"`
	Scope      *ScopeData             `json:"scope"`

	DroppedAttributesCount uint32 `json:"droppedAttributesCount"`
	DroppedEventsCount     uint32 `json:"droppedEventsCount"`
	DroppedLinksCount      uint32 `json:"droppedLinksCount"`

	StatusCode    string `json:"statusCode"`
	StatusMessage string `json:"statusMessage"`
}

type EventData struct {
	Name                   string                 `json:"name"`
	Timestamp              time.Time              `json:"timestamp"`
	Attributes             map[string]interface{} `json:"attributes"`
	DroppedAttributesCount uint32                 `json:"droppedAttributesCount"`
}

type LinkData struct {
	TraceID                string                 `json:"traceID"`
	SpanID                 string                 `json:"spanID"`
	TraceState             string                 `json:"traceState"`
	Attributes             map[string]interface{} `json:"attributes"`
	DroppedAttributesCount uint32                 `json:"droppedAttributesCount"`
}
