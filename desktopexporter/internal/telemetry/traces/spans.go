package traces

import (
	"encoding/json"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SpanPayload struct {
	Traces ptrace.Traces
}

type SpanData struct {
	TraceID      string `json:"traceID"`
	TraceState   string `json:"traceState"`
	SpanID       string `json:"spanID"`
	ParentSpanID string `json:"parentSpanID"`

	Name      string `json:"name"`
	Kind      string `json:"kind"`
	StartTime int64  `json:"-"`
	EndTime   int64  `json:"-"`

	Attributes attributes.Attributes  `json:"attributes"`
	Events     Events                 `json:"events"`
	Links      Links                  `json:"links"`
	Resource   *resource.ResourceData `json:"resource"`
	Scope      *scope.ScopeData       `json:"scope"`

	DroppedAttributesCount uint32 `json:"droppedAttributesCount"`
	DroppedEventsCount     uint32 `json:"droppedEventsCount"`
	DroppedLinksCount      uint32 `json:"droppedLinksCount"`

	StatusCode    string `json:"statusCode"`
	StatusMessage string `json:"statusMessage"`
}

func NewSpanPayload(t ptrace.Traces) *SpanPayload {
	return &SpanPayload{Traces: t}
}

func (payload *SpanPayload) ExtractSpans() []SpanData {
	spanSlice := []SpanData{}

	for _, resourceSpan := range payload.Traces.ResourceSpans().All() {
		resourceData := resource.AggregateResourceData(resourceSpan.Resource())

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeData := scope.AggregateScopeData(scopeSpan.Scope())

			for _, span := range scopeSpan.Spans().All() {
				eventsPayload := EventPayload{span.Events()}
				eventData := eventsPayload.extractEvents()

				linkPayload := LinkPayload{span.Links()}
				linkData := linkPayload.ExtractLinks()

				spanSlice = append(spanSlice, aggregateSpanData(span, eventData, linkData, scopeData, resourceData))
			}
		}
	}
	return spanSlice
}

func aggregateSpanData(source ptrace.Span, eventData []EventData, linkData []LinkData, scopeData *scope.ScopeData, resourceData *resource.ResourceData) SpanData {
	return SpanData{
		TraceID:    source.TraceID().String(),
		TraceState: source.TraceState().AsRaw(),

		SpanID:       source.SpanID().String(),
		ParentSpanID: source.ParentSpanID().String(),
		Name:         source.Name(),
		Kind:         source.Kind().String(),
		StartTime:    source.StartTimestamp().AsTime().UnixNano(),
		EndTime:      source.EndTimestamp().AsTime().UnixNano(),
		Attributes:   attributes.Attributes(source.Attributes().AsRaw()),

		Events:   Events(eventData),
		Links:    Links(linkData),
		Scope:    scopeData,
		Resource: resourceData,

		DroppedAttributesCount: source.DroppedAttributesCount(),
		DroppedEventsCount:     source.DroppedEventsCount(),
		DroppedLinksCount:      source.DroppedLinksCount(),

		StatusCode:    source.Status().Code().String(),
		StatusMessage: source.Status().Message(),
	}
}

// MarshalJSON implements custom JSON marshaling for SpanData
func (spanData SpanData) MarshalJSON() ([]byte, error) {
	type Alias SpanData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
	}{
		Alias:     Alias(spanData),
		StartTime: strconv.FormatInt(spanData.StartTime, 10),
		EndTime:   strconv.FormatInt(spanData.EndTime, 10),
	})
}
