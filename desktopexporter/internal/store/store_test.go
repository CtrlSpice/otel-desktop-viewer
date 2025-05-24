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
			Attributes:   map[string]any{},
			Events:       []telemetry.EventData{},
			Links:        []telemetry.LinkData{},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name": "test-service",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
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
			Attributes:   map[string]any{},
			Events:       []telemetry.EventData{},
			Links:        []telemetry.LinkData{},
			Resource: &telemetry.ResourceData{
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
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
				Attributes: map[string]any{
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
				Attributes: map[string]any{},
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
				Attributes: map[string]any{
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

func TestAttributeTypes(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	// Create a test span with all supported attribute types
	baseTime := time.Now()
	spans := []telemetry.SpanData{
		{
			TraceID:      "test-trace-1",
			SpanID:       "test-span-1",
			ParentSpanID: "",
			Name:         "test-span",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    baseTime,
			EndTime:      baseTime.Add(time.Second),
			Attributes: map[string]any{
				"string1":      "Koala",
				"string2":      "Bear",
				"bigint1":   int64(42),
				"double":   float64(3.14),
				"boolean":  true,
				"str_list": []string{"one", "two", "three"},
				"int_list": []int64{1, 2, 3},
				"float_list": []float64{1.1, 2.2, 3.3},
				"bool_list": []bool{true, false, true},
			},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:    "test-scope",
				Version: "v1.0.0",
				Attributes: map[string]any{},
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
	assert.NoError(t, err, "could not add spans")

	// Retrieve the trace
	trace, err := store.GetTrace(ctx, "test-trace-1")
	assert.NoError(t, err, "could not get trace")
	assert.Len(t, trace.Spans, 1, "expected 1 span")

	span := trace.Spans[0]

	// Verify each attribute type was stored and retrieved correctly
	assert.Equal(t, "Koala", span.Attributes["string1"])
	assert.Equal(t, "Bear", span.Attributes["string2"])
	assert.Equal(t, int64(42), span.Attributes["bigint1"])
	assert.Equal(t, float64(3.14), span.Attributes["double"])
	assert.Equal(t, true, span.Attributes["boolean"])
	assert.Equal(t, []string{"one", "two", "three"}, span.Attributes["str_list"])
	assert.Equal(t, []int64{1, 2, 3}, span.Attributes["int_list"])
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, span.Attributes["float_list"])
	assert.Equal(t, []bool{true, false, true}, span.Attributes["bool_list"])
}

func TestEventsAndLinks(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	// Create a test span with events and links containing different attribute types
	baseTime := time.Now()
	event1Time := baseTime.Add(100 * time.Millisecond)
	event2Time := baseTime.Add(200 * time.Millisecond)

	spans := []telemetry.SpanData{
		{
			TraceID:      "test-trace-events-links",
			SpanID:       "test-span-1",
			ParentSpanID: "",
			Name:         "test-span",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    baseTime,
			EndTime:      baseTime.Add(time.Second),
			Attributes: map[string]any{
				"string1": "Koala",
				"string2": "Bear",
			},
			Events: []telemetry.EventData{
				{
					Name:      "event1",
					Timestamp: event1Time,
					Attributes: map[string]any{
						"event_string": "Hello",
						"event_int":    int64(42),
						"event_float":  float64(3.14),
						"event_bool":   true,
						"event_list":   []string{"one", "two", "three"},
					},
					DroppedAttributesCount: 0,
				},
				{
					Name:      "event2",
					Timestamp: event2Time,
					Attributes: map[string]any{
						"event_string2": "World",
						"event_int2":    int64(100),
						"event_float2":  float64(6.28),
						"event_bool2":   false,
						"event_list2":   []int64{1, 2, 3, 4, 5},
					},
					DroppedAttributesCount: 1,
				},
			},
			Links: []telemetry.LinkData{
				{
					TraceID:    "linked-trace-1",
					SpanID:     "linked-span-1",
					TraceState: "state1",
					Attributes: map[string]any{
						"link_string": "Link1",
						"link_int":    int64(123),
						"link_float":  float64(9.99),
						"link_bool":   true,
						"link_list":   []float64{1.1, 2.2, 3.3},
					},
					DroppedAttributesCount: 0,
				},
				{
					TraceID:    "linked-trace-2",
					SpanID:     "linked-span-2",
					TraceState: "state2",
					Attributes: map[string]any{
						"link_string2": "Link2",
						"link_int2":    int64(456),
						"link_float2":  float64(12.34),
						"link_bool2":   false,
						"link_list2":   []bool{true, false, true},
					},
					DroppedAttributesCount: 2,
				},
			},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:    "test-scope",
				Version: "v1.0.0",
				Attributes: map[string]any{},
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
	assert.NoError(t, err, "could not add spans")

	// Retrieve the trace
	trace, err := store.GetTrace(ctx, "test-trace-events-links")
	assert.NoError(t, err, "could not get trace")
	assert.Len(t, trace.Spans, 1, "expected 1 span")

	span := trace.Spans[0]

	// Verify events
	assert.Len(t, span.Events, 2, "expected 2 events")
	
	// Check first event
	event1 := span.Events[0]
	assert.Equal(t, "event1", event1.Name)
	assert.Equal(t, event1Time.Format(time.RFC3339Nano), event1.Timestamp.Format(time.RFC3339Nano))
	assert.Equal(t, "Hello", event1.Attributes["event_string"])
	assert.Equal(t, int64(42), event1.Attributes["event_int"])
	assert.Equal(t, float64(3.14), event1.Attributes["event_float"])
	assert.Equal(t, true, event1.Attributes["event_bool"])
	assert.Equal(t, []string{"one", "two", "three"}, event1.Attributes["event_list"])
	assert.Equal(t, uint32(0), event1.DroppedAttributesCount)
	
	// Check second event
	event2 := span.Events[1]
	assert.Equal(t, "event2", event2.Name)
	assert.Equal(t, event2Time.Format(time.RFC3339Nano), event2.Timestamp.Format(time.RFC3339Nano))
	assert.Equal(t, "World", event2.Attributes["event_string2"])
	assert.Equal(t, int64(100), event2.Attributes["event_int2"])
	assert.Equal(t, float64(6.28), event2.Attributes["event_float2"])
	assert.Equal(t, false, event2.Attributes["event_bool2"])
	assert.Equal(t, []int64{1, 2, 3, 4, 5}, event2.Attributes["event_list2"])
	assert.Equal(t, uint32(1), event2.DroppedAttributesCount)
	
	// Verify links
	assert.Len(t, span.Links, 2, "expected 2 links")
	
	// Check first link
	link1 := span.Links[0]
	assert.Equal(t, "linked-trace-1", link1.TraceID)
	assert.Equal(t, "linked-span-1", link1.SpanID)
	assert.Equal(t, "state1", link1.TraceState)
	assert.Equal(t, "Link1", link1.Attributes["link_string"])
	assert.Equal(t, int64(123), link1.Attributes["link_int"])
	assert.Equal(t, float64(9.99), link1.Attributes["link_float"])
	assert.Equal(t, true, link1.Attributes["link_bool"])
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, link1.Attributes["link_list"])
	assert.Equal(t, uint32(0), link1.DroppedAttributesCount)
	
	// Check second link
	link2 := span.Links[1]
	assert.Equal(t, "linked-trace-2", link2.TraceID)
	assert.Equal(t, "linked-span-2", link2.SpanID)
	assert.Equal(t, "state2", link2.TraceState)
	assert.Equal(t, "Link2", link2.Attributes["link_string2"])
	assert.Equal(t, int64(456), link2.Attributes["link_int2"])
	assert.Equal(t, float64(12.34), link2.Attributes["link_float2"])
	assert.Equal(t, false, link2.Attributes["link_bool2"])
	assert.Equal(t, []bool{true, false, true}, link2.Attributes["link_list2"])
	assert.Equal(t, uint32(2), link2.DroppedAttributesCount)
}
