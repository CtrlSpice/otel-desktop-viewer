package telemetry

import (
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SpanPayload struct {
	traces ptrace.Traces
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

	Attributes map[string]any `json:"attributes"`
	Events     []EventData    `json:"events"`
	Links      []LinkData     `json:"links"`
	Resource   *ResourceData  `json:"resource"`
	Scope      *ScopeData     `json:"scope"`

	DroppedAttributesCount uint32 `json:"droppedAttributesCount"`
	DroppedEventsCount     uint32 `json:"droppedEventsCount"`
	DroppedLinksCount      uint32 `json:"droppedLinksCount"`

	StatusCode    string `json:"statusCode"`
	StatusMessage string `json:"statusMessage"`
}

func NewSpanPayload(t ptrace.Traces) *SpanPayload {
	return &SpanPayload{traces: t}
}

func (payload *SpanPayload) ExtractSpans() []SpanData {
	spanSlice := []SpanData{}

	for _, resourceSpan := range payload.traces.ResourceSpans().All() {
		resourceData := AggregateResourceData(resourceSpan.Resource())

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeData := AggregateScopeData(scopeSpan.Scope())

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

func aggregateSpanData(source ptrace.Span, eventData []EventData, linkData []LinkData, scopeData *ScopeData, resourceData *ResourceData) SpanData {
	return SpanData{
		TraceID:    source.TraceID().String(),
		TraceState: source.TraceState().AsRaw(),

		SpanID:       source.SpanID().String(),
		ParentSpanID: source.ParentSpanID().String(),
		Name:         source.Name(),
		Kind:         source.Kind().String(),
		StartTime:    source.StartTimestamp().AsTime().UnixNano(),
		EndTime:      source.EndTimestamp().AsTime().UnixNano(),
		Attributes:   source.Attributes().AsRaw(),

		Events:   eventData,
		Links:    linkData,
		Scope:    scopeData,
		Resource: resourceData,

		DroppedAttributesCount: source.DroppedAttributesCount(),
		DroppedEventsCount:     source.DroppedEventsCount(),
		DroppedLinksCount:      source.DroppedLinksCount(),

		StatusCode:    source.Status().Code().String(),
		StatusMessage: source.Status().Message(),
	}
}

// Get the service name of a span with respect to OTEL semanic conventions:
// service.name must be a string value having a meaning that helps to distinguish a group of services.
// Read more here: (https://opentelemetry.io/docs/reference/specification/resource/semantic_conventions/#service)
func (spanData *SpanData) GetServiceName() string {
	serviceName, ok := spanData.Resource.Attributes["service.name"]
	if !ok {
		fmt.Println("traceID:", spanData.TraceID, "spanID:", spanData.SpanID, ErrInvalidServiceName)
		return ""
	}
	return serviceName.(string)
}

// MarshalJSON implements custom JSON marshaling for SpanData
func (spanData SpanData) MarshalJSON() ([]byte, error) {
	type Alias SpanData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		StartTime PreciseTimestamp `json:"startTime"`
		EndTime   PreciseTimestamp `json:"endTime"`
	}{
		Alias:     Alias(spanData),
		StartTime: NewPreciseTimestamp(spanData.StartTime),
		EndTime:   NewPreciseTimestamp(spanData.EndTime),
	})
} 