package telemetry

import (
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
)

type RecentTelemetrySummaries struct {
	Summaries []TelemetrySummary `json:"summaries"`
}

type TelemetrySummary struct {
	ServiceName string `json:"serviceName"`
	Type        string `json:"type"`
	ID          string `json:"ID"`
}

type TelemetryData struct {
	ID     string     `json:"ID"`
	Type   string     `json:"type"`
	Metric MetricData `json:"metric"`
	Log    LogData    `json:"log"`
	Trace  TraceData  `json:"trace"`
}

type LogData struct {
	Body                   string                 `json:"body"`
	TraceID                string                 `json:"traceID"`
	SpanID                 string                 `json:"spanID"`
	Timestamp              time.Time              `json:"timestamp"`
	ObservedTimestamp      time.Time              `json:"observedTimestamp"`
	Attributes             map[string]interface{} `json:"attributes"`
	Resource               *ResourceData          `json:"resource"`
	Scope                  *ScopeData             `json:"scope"`
	SeverityText           string                 `json:"severityText"`
	SeverityNumber         plog.SeverityNumber    `json:"severityNumber"`
	DroppedAttributesCount uint32                 `json:"droppedAttributeCount"`
	Flags                  plog.LogRecordFlags    `json:"flags"`
}

type MetricData struct {
	Name     string        `json:"name"`
	Resource *ResourceData `json:"resource"`
	Scope    *ScopeData    `json:"scope"`
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

type RecentSummaries struct {
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

type TraceData struct {
	TraceID string     `json:"traceID"`
	Spans   []SpanData `json:"spans"`
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
