package desktopexporter

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type ResourceData struct {
	Attributes             map[string]interface{}
	DroppedAttributesCount uint32
}

type ScopeData struct {
	Name                   string
	Version                string
	Attributes             map[string]interface{}
	DroppedAttributesCount uint32
}

type SpanData struct {
	TraceID    string
	TraceState string

	SpanId       string
	ParentSpanID string
	Name         string
	Kind         string
	StartTime    time.Time
	EndTime      time.Time

	Attributes map[string]interface{}
	Events     []EventData
	Links      []LinkData
	Resource   *ResourceData
	Scope      *ScopeData

	DroppedAttributesCount uint32
	DroppedEventsCount     uint32
	DroppedLinksCount      uint32

	StatusCode    string
	StatusMessage string
}

type EventData struct {
	Name                   string
	Timestamp              time.Time
	Attributes             map[string]interface{}
	DroppedAttributesCount uint32
}

type LinkData struct {
	TraceID                string
	SpanID                 string
	TraceState             string
	Attributes             map[string]interface{}
	DroppedAttributesCount uint32
}

func extractSpans(_ context.Context, traces ptrace.Traces) []SpanData {
	extractedSpans := make([]SpanData, 0, traces.SpanCount())

	for rsi := 0; rsi < traces.ResourceSpans().Len(); rsi++ {
		resourceSpan := traces.ResourceSpans().At(rsi)
		resourceData := aggregateResourceData(resourceSpan.Resource())

		for ssi := 0; ssi < resourceSpan.ScopeSpans().Len(); ssi++ {
			scopeSpan := resourceSpan.ScopeSpans().At(ssi)
			scopeData := aggregateScopeData(scopeSpan.Scope())

			for si := 0; si < scopeSpan.Spans().Len(); si++ {
				span := scopeSpan.Spans().At(si)
				eventData := extractEvents(span.Events())
				linkData := extractLinks(span.Links())
				spanData := aggregateSpanData(span, eventData, linkData, scopeData, resourceData)
				extractedSpans = append(extractedSpans, spanData)
			}
		}
	}
	return extractedSpans
}

func extractEvents(events ptrace.SpanEventSlice) []EventData {
	eventDataSlice := make([]EventData, 0, events.Len())
	for eventIndex := 0; eventIndex < events.Len(); eventIndex++ {
		eventDataSlice = append(eventDataSlice, aggregateEventData(events.At(eventIndex)))
	}

	return eventDataSlice
}

func extractLinks(links ptrace.SpanLinkSlice) []LinkData {
	linkDataSlice := make([]LinkData, 0, links.Len())
	for linkIndex := 0; linkIndex < links.Len(); linkIndex++ {
		linkDataSlice = append(linkDataSlice, aggregateLinkData(links.At(linkIndex)))
	}

	return linkDataSlice
}

func aggregateResourceData(resource pcommon.Resource) *ResourceData {
	return &ResourceData{
		Attributes:             resource.Attributes().AsRaw(),
		DroppedAttributesCount: resource.DroppedAttributesCount(),
	}
}

func aggregateScopeData(scope pcommon.InstrumentationScope) *ScopeData {
	return &ScopeData{
		Name:                   scope.Name(),
		Version:                scope.Version(),
		Attributes:             scope.Attributes().AsRaw(),
		DroppedAttributesCount: scope.DroppedAttributesCount(),
	}
}

func aggregateEventData(event ptrace.SpanEvent) EventData {
	return EventData{
		Name:                   event.Name(),
		Timestamp:              event.Timestamp().AsTime(),
		Attributes:             event.Attributes().AsRaw(),
		DroppedAttributesCount: event.DroppedAttributesCount(),
	}
}

func aggregateLinkData(link ptrace.SpanLink) LinkData {
	return LinkData{
		TraceID:                link.TraceID().HexString(),
		SpanID:                 link.SpanID().HexString(),
		TraceState:             link.TraceState().AsRaw(),
		Attributes:             link.Attributes().AsRaw(),
		DroppedAttributesCount: link.DroppedAttributesCount(),
	}
}

func aggregateSpanData(span ptrace.Span, eventData []EventData, LinkData []LinkData, scopeData *ScopeData, resourceData *ResourceData) SpanData {
	return SpanData{
		TraceID:    span.TraceID().HexString(),
		TraceState: span.TraceState().AsRaw(),

		SpanId:       span.SpanID().HexString(),
		ParentSpanID: span.ParentSpanID().HexString(),
		Name:         span.Name(),
		Kind:         span.Kind().String(),
		StartTime:    span.StartTimestamp().AsTime(),
		EndTime:      span.EndTimestamp().AsTime(),
		Attributes:   span.Attributes().AsRaw(),

		Events:   eventData,
		Links:    LinkData,
		Scope:    scopeData,
		Resource: resourceData,

		DroppedAttributesCount: span.DroppedAttributesCount(),
		DroppedEventsCount:     span.DroppedEventsCount(),
		DroppedLinksCount:      span.DroppedLinksCount(),

		StatusCode:    span.Status().Code().String(),
		StatusMessage: span.Status().Message(),
	}
}
