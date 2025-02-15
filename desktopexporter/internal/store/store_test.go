package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestPersistence(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "./quack.db")

	// Check that db file is created properly
	_, err := os.Stat("./quack.db")
	assert.NoErrorf(t, err, "database file does not exist: %v", err)

	// Add sample spans to the store
	err = store.AddSpans(ctx, telemetry.NewSampleTelemetry().Spans)
	assert.NoErrorf(t, err, "could not add spans to the database: %v", err)

	// Get trace summaries and check length
	summaries, err := store.GetTraceSummaries(ctx)
	if assert.NoErrorf(t, err, "could not get trace summaries: %v", err) {
		assert.Len(t, summaries, 2)
	}

	// Close store
	err = store.Close()
	assert.NoErrorf(t, err, "could not close database: %v", err)

	// Reopen store from the database file
	store = NewStore(ctx, "./quack.db")

	// Get a trace by ID and check ID of root span
	trace, err := store.GetTrace(ctx, "42957c7c2fca940a0d32a0cdd38c06a4")
	if assert.NoErrorf(t, err, "could not get trace: %v", err) {
		assert.Equal(t, "37fd1349bf83d330", trace.Spans[0].SpanID)
	}

	// Clean up
	err = store.Close()
	assert.NoErrorf(t, err, "could not close database: %v", err)

	err = os.Remove("./quack.db")
	assert.NoError(t, err, "could not remove database file: %v", err)
}

func TestTracesWithoutRootSpans(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	// Create test spans: one trace with root span, one without
	spans := []telemetry.SpanData{
		{
			TraceID:      "trace1",
			SpanID:       "span1",
			ParentSpanID: "", // This is a root span
			Name:         "root span",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    time.Now(),
			EndTime:      time.Now().Add(time.Second),
			Attributes:   map[string]interface{}{},
			Events:       []telemetry.EventData{},
			Links:        []telemetry.LinkData{},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]interface{}{
					"service.name": "test-service",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]interface{}{},
				DroppedAttributesCount: 0,
			},
			DroppedAttributesCount: 0,
			DroppedEventsCount:     0,
			DroppedLinksCount:      0,
			StatusCode:             "STATUS_CODE_OK",
			StatusMessage:          "",
		},
		{
			TraceID:      "trace2",
			SpanID:       "span2",
			ParentSpanID: "some-missing-parent", // This is not a root span
			Name:         "child span",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    time.Now(),
			EndTime:      time.Now().Add(time.Second),
			Attributes:   map[string]interface{}{},
			Events:       []telemetry.EventData{},
			Links:        []telemetry.LinkData{},
			Resource: &telemetry.ResourceData{
				Attributes:             map[string]interface{}{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]interface{}{},
				DroppedAttributesCount: 0,
			},
			DroppedAttributesCount: 0,
			DroppedEventsCount:     0,
			DroppedLinksCount:      0,
			StatusCode:             "STATUS_CODE_OK",
			StatusMessage:          "",
		},
	}

	// Add spans to store
	err := store.AddSpans(ctx, spans)
	assert.NoError(t, err, "failed to add spans")

	// Get summaries
	summaries, err := store.GetTraceSummaries(ctx)
	assert.NoError(t, err, "failed to get trace summaries")

	// Should have two traces
	assert.Len(t, summaries, 2, "Expected 2 traces (one with root span, one without)")

	// Find each trace summary
	var trace1Summary, trace2Summary *telemetry.TraceSummary
	for _, summary := range summaries {
		if summary.TraceID == "trace1" {
			trace1Summary = &summary
		} else if summary.TraceID == "trace2" {
			trace2Summary = &summary
		}
	}

	// Verify trace with root span
	assert.NotNil(t, trace1Summary, "trace1 summary not found")
	assert.NotNil(t, trace1Summary.RootSpan, "trace1 should have root span")
	assert.Equal(t, "test-service", trace1Summary.RootSpan.ServiceName)
	assert.Equal(t, "root span", trace1Summary.RootSpan.Name)
	assert.Equal(t, uint32(1), trace1Summary.SpanCount)

	// Verify trace without root span
	assert.NotNil(t, trace2Summary, "trace2 summary not found")
	assert.Nil(t, trace2Summary.RootSpan, "trace2 should not have root span")
	assert.Equal(t, uint32(1), trace2Summary.SpanCount)
}

func TestTraceSummaryOrdering(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer func() {
		store.Close()
	}()

	baseTime := time.Now()

	// Create test spans with different timing scenarios
	spans := []telemetry.SpanData{
		{
			// Trace 1: Middle time
			TraceID:      "trace1",
			SpanID:       "span1",
			ParentSpanID: "", // root span
			Name:         "root middle",
			StartTime:    baseTime.Add(time.Second), // t+1
			EndTime:      baseTime.Add(2 * time.Second),
			Resource: &telemetry.ResourceData{
				Attributes: map[string]interface{}{
					"service.name": "service1",
				},
			},
			Scope: &telemetry.ScopeData{},
		},
		{
			// Trace 2: Oldest time
			TraceID:      "trace2",
			SpanID:       "span2",
			ParentSpanID: "missing_parent",
			Name:         "earliest no root",
			StartTime:    baseTime, // t+0
			EndTime:      baseTime.Add(2 * time.Second),
			Resource: &telemetry.ResourceData{
				Attributes: map[string]interface{}{},
			},
			Scope: &telemetry.ScopeData{},
		},
		{
			// Trace 3: Newest time
			TraceID:      "trace3",
			SpanID:       "span3",
			ParentSpanID: "", // root span
			Name:         "root last",
			StartTime:    baseTime.Add(2 * time.Second), // t+2
			EndTime:      baseTime.Add(3 * time.Second),
			Resource: &telemetry.ResourceData{
				Attributes: map[string]interface{}{
					"service.name": "service3",
				},
			},
			Scope: &telemetry.ScopeData{},
		},
	}

	// Add spans to store
	err := store.AddSpans(ctx, spans)
	assert.NoError(t, err, "failed to add spans")

	// Get summaries
	summaries, err := store.GetTraceSummaries(ctx)
	assert.NoError(t, err, "failed to get trace summaries")

	// Should have all three traces
	assert.Len(t, summaries, 3, "expected 3 traces")

	// Log the actual ordering we got
	t.Logf("Trace order: %s -> %s -> %s",
		summaries[0].TraceID,
		summaries[1].TraceID,
		summaries[2].TraceID)

	// Verify ordering: trace3 (newest) -> trace1 -> trace2 (oldest)
	assert.Equal(t, "trace3", summaries[0].TraceID, "first trace should be trace3 (latest start)")
	assert.Equal(t, "trace1", summaries[1].TraceID, "second trace should be trace1")
	assert.Equal(t, "trace2", summaries[2].TraceID, "last trace should be trace2 (earliest start)")

	// Verify root span presence
	assert.Nil(t, summaries[2].RootSpan, "trace2 should not have root span")
	assert.NotNil(t, summaries[1].RootSpan, "trace1 should have root span")
	assert.NotNil(t, summaries[0].RootSpan, "trace3 should have root span")
}
