package telemetry

import (
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type RecentTelemetrySummaries struct {
	Summaries []TelemetrySummary `json:"summaries"`
}

type TelemetrySummary struct {
	HasRootSpan bool `json:"hasRootSpan"`

	RootServiceName string    `json:"rootServiceName"`
	RootName        string    `json:"rootName"`
	RootStartTime   time.Time `json:"rootStartTime"`
	RootEndTime     time.Time `json:"rootEndTime"`
	ServiceName     string    `json:"serviceName"`

	SpanCount uint32 `json:"spanCount"`
	ID        string `json:"traceID"`
	Type      string `json:"type"`
}

type TelemetryData struct {
	ID     string     `json:"ID"`
	Type   string     `json:"type"`
	Metric MetricData `json:"metric,omitempty"`
	Log    LogData    `json:"log,omitempty"`
	Trace  TraceData  `json:"trace,omitempty"`
}

type Unique interface {
	ID() string
}

type LogData struct {
	Body                   string                 `json:"body,omitempty"`
	TraceID                string                 `json:"traceID,omitempty"`
	SpanID                 string                 `json:"spanID,omitempty"`
	Timestamp              time.Time              `json:"timestamp,omitempty"`
	ObservedTimestamp      time.Time              `json:"observedTimestamp,omitempty"`
	Attributes             map[string]interface{} `json:"attributes,omitempty"`
	SeverityText           string                 `json:"severityText,omitempty"`
	SeverityNumber         plog.SeverityNumber    `json:"severityNumber,omitempty"`
	DroppedAttributesCount uint32                 `json:"droppedAttributeCount,omitempty"`
	Flags                  plog.LogRecordFlags    `json:"flags,omitempty"`
	Resource               *ResourceData          `json:"resource"`
	Scope                  *ScopeData             `json:"scope"`
}

func (l LogData) ID() string {
	// may need to consider additional fields to uniquely identify
	// a log, for example different resources could potentially
	// send the same data at the same time and create collisions
	return l.Body + l.Timestamp.String() + l.ObservedTimestamp.String()
}

type MetricData struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Unit        string             `json:"unit,omitempty"`
	Type        pmetric.MetricType `json:"type,omitempty"`
	// add datapoints
	Resource *ResourceData `json:"resource"`
	Scope    *ScopeData    `json:"scope"`
	Received time.Time     `json:"-"`
}

func (m MetricData) ID() string {
	// may need to consider additional fields to uniquely identify
	// a metric, for example different resources could potentially
	// send the same data at the same time and create collisions
	return m.Name + m.Received.String()
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
