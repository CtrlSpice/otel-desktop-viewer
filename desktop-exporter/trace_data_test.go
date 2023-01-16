package desktopexporter

import (
	"context"
	"testing"
	"time"

	"github.com/CtrlSpice/desktop-collector/desktop-exporter/testdata"
	"github.com/stretchr/testify/assert"
)

func TestGetTraceSummaryWithRootSpan(t *testing.T) {
	maxQueueLength := 1
	spansPerTrace := 3

	traces := testdata.GenerateOTLPPayload(1, 1, maxQueueLength*spansPerTrace)
	ctx := context.Background()
	store := NewTraceStore(maxQueueLength)
	spans := extractSpans(ctx, traces)

	for i, span := range spans {
		span.TraceID = "1"
		span.SpanID = string(rune(i));
		// make 0 the root span.
		if i == 0 {
			span.Name = "rootSpan"
			span.ParentSpanID = ""
			span.Resource.Attributes["service.name"] = "service name"
		} else {
			span.ParentSpanID = "0";
		}
		store.Add(ctx, span)
	}

	trace, err := store.GetTrace("1")
	assert.NoError(t, err)

	summary := trace.GetTraceSummary()
	assert.True(t, summary.HasRootSpan)
	assert.Equal(t, "service name", summary.RootServiceName)
	assert.Equal(t, "rootSpan", summary.RootName)
	assert.Equal(t, time.Date(2022, 10, 21, 7, 10, 2, 100, time.UTC), summary.RootStartTime)
	assert.Equal(t, time.Date(2020, 10, 21, 7, 10, 2, 300, time.UTC), summary.RootEndTime)
	assert.Equal(t, uint32(spansPerTrace), summary.SpanCount)
	assert.Equal(t, "1", summary.TraceID)
}

func TestGetTraceSummaryMissingRootSpan(t *testing.T) {
	maxQueueLength := 1
	spansPerTrace := 3

	traces := testdata.GenerateOTLPPayload(1, 1, maxQueueLength*spansPerTrace)
	ctx := context.Background()
	store := NewTraceStore(maxQueueLength)
	spans := extractSpans(ctx, traces)

	for _, span := range spans {
		span.TraceID = "1"
		store.Add(ctx, span)
	}

	trace, err := store.GetTrace("1")
	assert.NoError(t, err)

	summary := trace.GetTraceSummary()
	assert.False(t, summary.HasRootSpan)
	assert.Equal(t, "", summary.RootServiceName)
	assert.Equal(t, "", summary.RootName)
	assert.True(t, time.Time.IsZero(summary.RootStartTime))
	assert.True(t, time.Time.IsZero(summary.RootEndTime))
	assert.Equal(t, uint32(spansPerTrace), summary.SpanCount)
	assert.Equal(t, "1", summary.TraceID)
}