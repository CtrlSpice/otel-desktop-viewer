package desktopexporter

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/CtrlSpice/desktop-collector/desktop-exporter/testdata"

	"github.com/stretchr/testify/assert"
)

const (
	resourceCount = 2
	scopeCount    = 3
	spanCount     = 4
)

func TestExtractSpans(t *testing.T) {
	traces := testdata.GenerateTraces(resourceCount, scopeCount, spanCount)
	spans := extractSpans(context.Background(), traces)

	// Validate static resource data
	assert.Equal(t, "resource attribute value", spans[0].Resource.Attributes["resource attribute"])
	assert.Equal(t, uint32(1), spans[0].Resource.DroppedAttributesCount)

	// Validate static instrumentation scope data
	assert.Equal(t, "instrumentational scope", spans[0].Scope.Name)
	assert.Equal(t, "v0.0.1", spans[0].Scope.Version)
	assert.Equal(t, uint32(2), spans[0].Scope.DroppedAttributesCount)

	// Validate static span data
	expTraceID := hex.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10})
	expSpanID := hex.EncodeToString([]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18})
	expParentSpanID := hex.EncodeToString([]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28})
	expStartTime := time.Date(2022, 10, 21, 7, 10, 2, 100, time.UTC)
	expEndTime := time.Date(2020, 10, 21, 7, 10, 2, 300, time.UTC)

	assert.Equal(t, expTraceID, spans[0].TraceID)
	assert.Equal(t, expSpanID, spans[0].SpanId)
	assert.Equal(t, expParentSpanID, spans[0].ParentSpanID)
	assert.Equal(t, "span", spans[0].Name)
	assert.Equal(t, expStartTime, spans[0].StartTime)
	assert.Equal(t, expEndTime, spans[0].EndTime)

	// Validate that the correct resource and instrumentation scope is attached to each span
	for i, span := range spans {
		expResourceIndex := i / (scopeCount * spanCount)
		expScopeIndex := (i - expResourceIndex*scopeCount*spanCount) / spanCount
		expSpanIndex := (i - expResourceIndex*scopeCount*spanCount - expScopeIndex*spanCount)

		assert.Equal(t, int64(expResourceIndex), span.Resource.Attributes["resource index"])
		assert.Equal(t, int64(expScopeIndex), span.Scope.Attributes["scope index"])
		assert.Equal(t, int64(expSpanIndex), span.Attributes["span index"])
	}
}
