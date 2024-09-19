package telemetry

import (
	"fmt"
	"time"

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

func (payload *SpanPayload) ExtractSpans() []SpanData {
	spanSlice := []SpanData{}

	for rsi := 0; rsi < payload.traces.ResourceSpans().Len(); rsi++ {
		resourceSpan := payload.traces.ResourceSpans().At(rsi)
		resourceData := AggregateResourceData(resourceSpan.Resource())

		for ssi := 0; ssi < resourceSpan.ScopeSpans().Len(); ssi++ {
			scopeSpan := resourceSpan.ScopeSpans().At(ssi)
			scopeData := AggregateScopeData(scopeSpan.Scope())

			for si := 0; si < scopeSpan.Spans().Len(); si++ {
				span := scopeSpan.Spans().At(si)

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

func aggregateSpanData(source ptrace.Span, eventData []EventData, LinkData []LinkData, scopeData *ScopeData, resourceData *ResourceData) SpanData {
	return SpanData{
		TraceID:    source.TraceID().String(),
		TraceState: source.TraceState().AsRaw(),

		SpanID:       source.SpanID().String(),
		ParentSpanID: source.ParentSpanID().String(),
		Name:         source.Name(),
		Kind:         source.Kind().String(),
		StartTime:    source.StartTimestamp().AsTime(),
		EndTime:      source.EndTimestamp().AsTime(),
		Attributes:   source.Attributes().AsRaw(),

		Events:   eventData,
		Links:    LinkData,
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
	serviceName, ok := spanData.Resource.Attributes["service.name"].(string)
	if !ok {
		fmt.Println("traceID:", spanData.TraceID, "spanID:", spanData.SpanID, ErrInvalidServiceName)
		return ""
	}
	return serviceName
}
