package testdata

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	spanStartTimestamp = pcommon.NewTimestampFromTime(time.Date(2022, 10, 21, 7, 10, 2, 100, time.UTC))
	spanEventTimestamp = pcommon.NewTimestampFromTime(time.Date(2020, 10, 21, 7, 10, 2, 150, time.UTC))
	spanEndTimestamp   = pcommon.NewTimestampFromTime(time.Date(2020, 10, 21, 7, 10, 2, 300, time.UTC))
)

func GenerateTraces(resourceCount, scopeCount, spanCount int) ptrace.Traces {
	traceData := ptrace.NewTraces()

	// Create and populate resource data
	traceData.ResourceSpans().EnsureCapacity(resourceCount)
	for resourceIndex := 0; resourceIndex < resourceCount; resourceIndex++ {
		resourceSpan := traceData.ResourceSpans().AppendEmpty()
		fillResource(resourceSpan.Resource(), resourceIndex)

		// Create and populate instrumentation scope data
		resourceSpan.ScopeSpans().EnsureCapacity(scopeCount)
		for scopeIndex := 0; scopeIndex < scopeCount; scopeIndex++ {
			scopeSpan := resourceSpan.ScopeSpans().AppendEmpty()
			fillScope(scopeSpan.Scope(), scopeIndex)

			//Create and populate spans
			scopeSpan.Spans().EnsureCapacity(spanCount)
			for spanIndex := 0; spanIndex < spanCount; spanIndex++ {
				span := scopeSpan.Spans().AppendEmpty()
				fillSpan(span, spanIndex)
			}
		}
	}

	return traceData
}

func fillResource(resource pcommon.Resource, resourceIndex int) {
	resource.SetDroppedAttributesCount(1)
	resource.Attributes().PutStr("resource attribute", "resource attribute value")
	resource.Attributes().PutInt("resource index", int64(resourceIndex))
}

func fillScope(scope pcommon.InstrumentationScope, scopeIndex int) {
	scope.SetDroppedAttributesCount(2)
	scope.SetName("instrumentational scope")
	scope.SetVersion("v0.0.1")
	scope.Attributes().PutInt("scope index", int64(scopeIndex))
}

func fillSpan(span ptrace.Span, spanIndex int) {
	span.SetName("span")
	span.SetStartTimestamp(spanStartTimestamp)
	span.SetEndTimestamp(spanEndTimestamp)
	span.SetDroppedAttributesCount(3)
	span.SetTraceID([16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10})
	span.SetSpanID([8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18})
	span.Attributes().PutInt("span index", int64(spanIndex))
	span.SetDroppedAttributesCount(3)
	span.SetDroppedEventsCount(4)
	span.SetDroppedLinksCount(5)

	event := span.Events().AppendEmpty()
	event.SetTimestamp(spanEventTimestamp)
	event.SetName("span event")
	event.Attributes().PutStr("span event attribute", "span event attribute value")
	event.SetDroppedAttributesCount(6)

	link := span.Links().AppendEmpty()
	link.Attributes().PutStr("span link attribute", "span link attribute value")
	link.SetDroppedAttributesCount(7)

	status := span.Status()
	status.SetCode(ptrace.StatusCodeOk)
	status.SetMessage("status ok")
}
