package desktopexporter

import (
	"context"
	"testing"
	"time"

	"github.com/CtrlSpice/desktop-collector/desktop-exporter/testdata"
	"github.com/stretchr/testify/assert"
)

func TestGetTraceSummary(t *testing.T) {
	maxQueueLength := 1
	spansPerTrace := 3

	traces := testdata.GenerateOTLPPayload(1, 1, maxQueueLength*spansPerTrace)
	ctx := context.Background()
	store := NewTraceStore(maxQueueLength)
	spans := extractSpans(ctx, traces)

	// Assign each span start and end time before adding it to the store
	// These timestamps are used to validate the summary's durationMS
	for i, span := range spans {
		span.TraceID = "1"
		span.StartTime = time.Date(2022, 10, 10, 0, 0, i, 0, time.UTC)
		span.EndTime = time.Date(2022, 10, 10, 0, 0, i+1, 0, time.UTC)
		store.Add(ctx, span)
	}

	trace := store.traceMap["1"]
	summary, err := trace.GetTraceSummary()
	assert.NoError(t, err)
	assert.Equal(t, "1", summary.TraceID)
	assert.Equal(t, uint32(spansPerTrace), summary.SpanCount)
	assert.Equal(t, int64(3000), summary.DurationMS)
}
