package desktopexporter

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktop-exporter/testdata"

	"github.com/stretchr/testify/assert"
)

const (
	resourceCount = 2
	scopeCount    = 3
	spanCount     = 4
)

func TestExtractSpans(t *testing.T) {
	traces := testdata.GenerateOTLPPayload(resourceCount, scopeCount, spanCount)
	spans := extractSpans(context.Background(), traces)

	// Validate number of spans extraced from traces object
	assert.Len(t, spans, traces.SpanCount())

	// Validate static resource data
	assert.Equal(t, "resource attribute value", spans[0].Resource.Attributes["resource attribute"])
	assert.Equal(t, uint32(1), spans[0].Resource.DroppedAttributesCount)

	// Validate static instrumentation scope data
	assert.Equal(t, "instrumentational scope", spans[0].Scope.Name)
	assert.Equal(t, "v0.0.1", spans[0].Scope.Version)
	assert.Equal(t, uint32(2), spans[0].Scope.DroppedAttributesCount)

	// Validate static span data
	expectedTraceID := hex.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10})
	expectedSpanID := hex.EncodeToString([]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18})
	expectedParentSpanID := hex.EncodeToString([]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28})
	expectedStartTime := time.Date(2022, 10, 21, 7, 10, 2, 100, time.UTC)
	expectedEndTime := time.Date(2020, 10, 21, 7, 10, 2, 300, time.UTC)
	expectedEventTime := time.Date(2020, 10, 21, 7, 10, 2, 150, time.UTC)

	assert.Equal(t, expectedTraceID, spans[0].TraceID)
	assert.Equal(t, expectedSpanID, spans[0].SpanID)
	assert.Equal(t, expectedParentSpanID, spans[0].ParentSpanID)
	assert.Equal(t, "span", spans[0].Name)
	assert.Equal(t, "SPAN_KIND_INTERNAL", spans[0].Kind)
	assert.Equal(t, expectedStartTime, spans[0].StartTime)
	assert.Equal(t, expectedEndTime, spans[0].EndTime)
	assert.Equal(t, uint32(3), spans[0].DroppedAttributesCount)
	assert.Equal(t, uint32(4), spans[0].DroppedEventsCount)
	assert.Equal(t, uint32(5), spans[0].DroppedLinksCount)
	assert.Equal(t, "STATUS_CODE_OK", spans[0].StatusCode)
	assert.Equal(t, "status ok", spans[0].StatusMessage)

	// Validate static event data
	assert.Equal(t, "span event", spans[0].Events[0].Name)
	assert.Equal(t, expectedEventTime, spans[0].Events[0].Timestamp)
	assert.Equal(t, "span event attribute value", spans[0].Events[0].Attributes["span event attribute"])
	assert.Equal(t, uint32(6), spans[0].Events[0].DroppedAttributesCount)

	//Validate static link data
	assert.Equal(t, expectedTraceID, spans[0].Links[0].TraceID)
	assert.Equal(t, "span link attribute value", spans[0].Links[0].Attributes["span link attribute"])
	assert.Equal(t, uint32(7), spans[0].Links[0].DroppedAttributesCount)

	// Validate that the correct resource and instrumentation scope is attached to each span
	for i, span := range spans {
		expectedResourceIndex := i / (scopeCount * spanCount)
		expectedScopeIndex := (i - expectedResourceIndex*scopeCount*spanCount) / spanCount
		expectedSpanIndex := (i - expectedResourceIndex*scopeCount*spanCount - expectedScopeIndex*spanCount)

		assert.Equal(t, int64(expectedResourceIndex), span.Resource.Attributes["resource index"])
		assert.Equal(t, int64(expectedScopeIndex), span.Scope.Attributes["scope index"])
		assert.Equal(t, int64(expectedSpanIndex), span.Attributes["span index"])
	}
}
