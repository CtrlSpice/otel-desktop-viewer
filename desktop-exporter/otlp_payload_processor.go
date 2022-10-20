package desktopexporter

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SpanData struct {
	TraceID      string
	SpanId       string
	ParentSpanID string
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Resource     *ResourceData
	Scope        *ScopeData
}

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
				spanData := aggregateSpanData(span, scopeData, resourceData)
				extractedSpans = append(extractedSpans, spanData)
			}
		}
	}
	return extractedSpans
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

// TODO: Complete Span Data with all relevant fields
func aggregateSpanData(span ptrace.Span, scopeData *ScopeData, resourceData *ResourceData) SpanData {
	return SpanData{
		TraceID:      span.TraceID().HexString(),
		SpanId:       span.SpanID().HexString(),
		ParentSpanID: span.ParentSpanID().HexString(),
		Name:         span.Name(),
		StartTime:    span.StartTimestamp().AsTime(),
		EndTime:      span.EndTimestamp().AsTime(),
		Scope:        scopeData,
		Resource:     resourceData,
	}
}
