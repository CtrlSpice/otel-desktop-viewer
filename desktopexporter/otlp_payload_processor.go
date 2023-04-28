package desktopexporter

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

func extractMetrics(metrics pmetric.Metrics) []telemetry.MetricData {
	md := []telemetry.MetricData{}

	for rsi := 0; rsi < metrics.ResourceMetrics().Len(); rsi++ {
		rm := metrics.ResourceMetrics().At(rsi)
		rd := aggregateResourceData(rm.Resource())

		for ssi := 0; ssi < rm.ScopeMetrics().Len(); ssi++ {
			sm := rm.ScopeMetrics().At(ssi)
			sd := aggregateScopeData(sm.Scope())

			for si := 0; si < sm.Metrics().Len(); si++ {
				metric := sm.Metrics().At(si)
				metricData := aggregateMetricData(metric, sd, rd)
				md = append(md, metricData)
			}
		}
	}
	return md
}

func extractLogs(logs plog.Logs) []telemetry.LogData {
	logData := []telemetry.LogData{}

	for rsi := 0; rsi < logs.ResourceLogs().Len(); rsi++ {
		rl := logs.ResourceLogs().At(rsi)
		rd := aggregateResourceData(rl.Resource())

		for ssi := 0; ssi < rl.ScopeLogs().Len(); ssi++ {
			sl := rl.ScopeLogs().At(ssi)
			sd := aggregateScopeData(sl.Scope())

			for si := 0; si < sl.LogRecords().Len(); si++ {
				log := sl.LogRecords().At(si)
				logData = append(logData, aggregateLogData(log, sd, rd))
			}
		}
	}
	return logData
}

func extractSpans(_ context.Context, traces ptrace.Traces) []telemetry.SpanData {
	extractedSpans := make([]telemetry.SpanData, 0, traces.SpanCount())

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

func extractEvents(events ptrace.SpanEventSlice) []telemetry.EventData {
	eventDataSlice := make([]telemetry.EventData, 0, events.Len())
	for eventIndex := 0; eventIndex < events.Len(); eventIndex++ {
		eventDataSlice = append(eventDataSlice, aggregateEventData(events.At(eventIndex)))
	}

	return eventDataSlice
}

func extractLinks(links ptrace.SpanLinkSlice) []telemetry.LinkData {
	linkDataSlice := make([]telemetry.LinkData, 0, links.Len())
	for linkIndex := 0; linkIndex < links.Len(); linkIndex++ {
		linkDataSlice = append(linkDataSlice, aggregateLinkData(links.At(linkIndex)))
	}

	return linkDataSlice
}

func aggregateResourceData(resource pcommon.Resource) *telemetry.ResourceData {
	return &telemetry.ResourceData{
		Attributes:             resource.Attributes().AsRaw(),
		DroppedAttributesCount: resource.DroppedAttributesCount(),
	}
}

func aggregateScopeData(scope pcommon.InstrumentationScope) *telemetry.ScopeData {
	return &telemetry.ScopeData{
		Name:                   scope.Name(),
		Version:                scope.Version(),
		Attributes:             scope.Attributes().AsRaw(),
		DroppedAttributesCount: scope.DroppedAttributesCount(),
	}
}

func aggregateEventData(event ptrace.SpanEvent) telemetry.EventData {
	return telemetry.EventData{
		Name:                   event.Name(),
		Timestamp:              event.Timestamp().AsTime(),
		Attributes:             event.Attributes().AsRaw(),
		DroppedAttributesCount: event.DroppedAttributesCount(),
	}
}

func aggregateLinkData(link ptrace.SpanLink) telemetry.LinkData {
	return telemetry.LinkData{
		TraceID:                link.TraceID().String(),
		SpanID:                 link.SpanID().String(),
		TraceState:             link.TraceState().AsRaw(),
		Attributes:             link.Attributes().AsRaw(),
		DroppedAttributesCount: link.DroppedAttributesCount(),
	}
}

func aggregateSpanData(span ptrace.Span, eventData []telemetry.EventData, LinkData []telemetry.LinkData, scopeData *telemetry.ScopeData, resourceData *telemetry.ResourceData) telemetry.SpanData {
	return telemetry.SpanData{
		TraceID:    span.TraceID().String(),
		TraceState: span.TraceState().AsRaw(),

		SpanID:       span.SpanID().String(),
		ParentSpanID: span.ParentSpanID().String(),
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

func aggregateMetricData(source pmetric.Metric, scope *telemetry.ScopeData, resource *telemetry.ResourceData) telemetry.MetricData {
	return telemetry.MetricData{
		Name: source.Name(),
		// TODO: add other fields
		Resource: resource,
		Scope:    scope,
	}
}

func aggregateLogData(source plog.LogRecord, scope *telemetry.ScopeData, resource *telemetry.ResourceData) telemetry.LogData {
	return telemetry.LogData{
		Body:                   source.Body().AsString(),
		TraceID:                source.TraceID().String(),
		SpanID:                 source.SpanID().String(),
		ObservedTimestamp:      source.ObservedTimestamp().AsTime(),
		Timestamp:              source.Timestamp().AsTime(),
		Attributes:             source.Attributes().AsRaw(),
		Resource:               resource,
		Scope:                  scope,
		DroppedAttributesCount: source.DroppedAttributesCount(),
		SeverityText:           source.SeverityText(),
		SeverityNumber:         source.SeverityNumber(),
		Flags:                  source.Flags(),
	}
}
