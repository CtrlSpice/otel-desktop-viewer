package store

import (
	"context"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

// testHelper holds common test dependencies
type testHelper struct {
	t     *testing.T
	ctx   context.Context
	store *Store
}

// setupTest creates a new test helper and returns a teardown function
func setupTest(t *testing.T) (*testHelper, func()) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	
	assert.NotNil(t, store, "store should not be nil")
	assert.NotNil(t, store.db, "database connection should not be nil")
	assert.NotNil(t, store.conn, "duckdb connection should not be nil")
	
	helper := &testHelper{
		t:     t,
		ctx:   ctx,
		store: store,
	}
	
	teardown := func() {
		if helper.store != nil {
			helper.store.Close()
		}
	}
	
	return helper, teardown
}

// TestTraceSummaryOrdering verifies that trace summaries are ordered by start time (newest first).
func TestTraceSummaryOrdering(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()

	// Create test spans with different timing scenarios
	spans := []telemetry.SpanData{
		{
			// Trace 1: Middle time
			TraceID:      "trace1",
			SpanID:       "span1",
			ParentSpanID: "", // root span
			Name:         "root middle",
			StartTime:    baseTime + time.Second.Nanoseconds(), // t+1
			EndTime:      baseTime + 2 * time.Second.Nanoseconds(),
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
			EndTime:      baseTime + 2 * time.Second.Nanoseconds(),
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
			StartTime:    baseTime + 2 * time.Second.Nanoseconds(), // t+2
			EndTime:      baseTime + 3 * time.Second.Nanoseconds(),
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name": "service3",
				},
			},
			Scope: &telemetry.ScopeData{},
		},
	}

	// Add spans to store
	err := helper.store.AddSpans(helper.ctx, spans)
	assert.NoError(t, err, "failed to add spans")

	// Get summaries
	summaries, err := helper.store.GetTraceSummaries(helper.ctx)
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

// TestTraceNotFound verifies error handling for non-existent trace IDs.
func TestTraceNotFound(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	_, err := helper.store.GetTrace(helper.ctx, "non-existent-trace")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTraceIDNotFound)
}

// TestEmptySpans verifies handling of empty span lists and empty stores.
func TestEmptySpans(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	// Test adding empty span list
	err := helper.store.AddSpans(helper.ctx, []telemetry.SpanData{})
	assert.NoError(t, err)

	// Test getting summaries from empty store
	summaries, err := helper.store.GetTraceSummaries(helper.ctx)
	assert.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestClearTraces verifies that all traces can be cleared from the store.
func TestClearTraces(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	// Add test trace
	spans := createTestTrace()
	err := helper.store.AddSpans(helper.ctx, spans)
	assert.NoError(t, err)

	// Verify trace exists
	summaries, err := helper.store.GetTraceSummaries(helper.ctx)
	assert.NoError(t, err)
	assert.Len(t, summaries, 1)

	// Clear traces
	err = helper.store.ClearTraces(helper.ctx)
	assert.NoError(t, err)

	// Verify store is empty
	summaries, err = helper.store.GetTraceSummaries(helper.ctx)
	assert.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestTraceSuite runs a comprehensive suite of tests on a single trace.
func TestTraceSuite(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	// Add the test trace
	spans := createTestTrace()
	err := helper.store.AddSpans(helper.ctx, spans)
	assert.NoError(t, err, "failed to add test trace")

	// Run all test cases
	t.Run("TraceSummary", func(t *testing.T) {
		summaries, err := helper.store.GetTraceSummaries(helper.ctx)
		assert.NoError(t, err)
		assert.Len(t, summaries, 1, "should have one trace summary")

		summary := summaries[0]
		assert.Equal(t, "test-trace", summary.TraceID)
		assert.Equal(t, uint32(3), summary.SpanCount)
		assert.NotNil(t, summary.RootSpan)
		assert.Equal(t, "test-service", summary.RootSpan.ServiceName)
		assert.Equal(t, "root-operation", summary.RootSpan.Name)
	})

	t.Run("TraceContent", func(t *testing.T) {
		trace, err := helper.store.GetTrace(helper.ctx, "test-trace")
		assert.NoError(t, err)
		assert.Len(t, trace.Spans, 3, "should have three spans")

		// Verify spans are ordered by start time
		assert.Equal(t, "root-span", trace.Spans[0].SpanID)
		assert.Equal(t, "child-span", trace.Spans[1].SpanID)
		assert.Equal(t, "orphaned-span", trace.Spans[2].SpanID)

		// Verify root span
		rootSpan := trace.Spans[0]
		assert.Empty(t, rootSpan.ParentSpanID)
		assert.Equal(t, "STATUS_CODE_OK", rootSpan.StatusCode)
		assert.Len(t, rootSpan.Events, 2)
		assert.Len(t, rootSpan.Links, 1)
		assert.Equal(t, "test-service", rootSpan.Resource.Attributes["service.name"])

		// Verify child span
		childSpan := trace.Spans[1]
		assert.Equal(t, "root-span", childSpan.ParentSpanID)
		assert.Equal(t, "STATUS_CODE_ERROR", childSpan.StatusCode)
		assert.Equal(t, "operation failed", childSpan.StatusMessage)
		assert.Len(t, childSpan.Events, 1)
		assert.Len(t, childSpan.Links, 1)

		// Verify orphaned span
		orphanedSpan := trace.Spans[2]
		assert.Equal(t, "non-existent-parent", orphanedSpan.ParentSpanID)
		assert.Equal(t, "STATUS_CODE_UNSET", orphanedSpan.StatusCode)
		assert.Empty(t, orphanedSpan.Events)
		assert.Empty(t, orphanedSpan.Links)
	})

	t.Run("TraceAttributes", func(t *testing.T) {
		trace, err := helper.store.GetTrace(helper.ctx, "test-trace")
		assert.NoError(t, err)

		// Verify root span attributes
		rootSpan := trace.Spans[0]
		assert.Equal(t, "root-value", rootSpan.Attributes["root.string"])
		assert.Equal(t, int64(42), rootSpan.Attributes["root.int"])
		assert.Equal(t, float64(3.14), rootSpan.Attributes["root.float"])
		assert.Equal(t, true, rootSpan.Attributes["root.bool"])
		rootList := rootSpan.Attributes["root.list"].([]any)
		assert.Equal(t, []any{"one", "two", "three"}, rootList)

		// Verify child span attributes
		childSpan := trace.Spans[1]
		assert.Equal(t, "child-value", childSpan.Attributes["child.string"])
		assert.Equal(t, int64(24), childSpan.Attributes["child.int"])
		assert.Equal(t, float64(2.71), childSpan.Attributes["child.float"])
		assert.Equal(t, false, childSpan.Attributes["child.bool"])
		childList := childSpan.Attributes["child.list"].([]any)
		assert.Equal(t, []any{int64(1), int64(2), int64(3), int64(4), int64(5)}, childList)
	})

	t.Run("TraceEventsAndLinks", func(t *testing.T) {
		trace, err := helper.store.GetTrace(helper.ctx, "test-trace")
		assert.NoError(t, err)

		// Verify root span events
		rootSpan := trace.Spans[0]
		assert.Equal(t, "root-event-1", rootSpan.Events[0].Name)
		assert.Equal(t, "Hello", rootSpan.Events[0].Attributes["event.string"])
		assert.Equal(t, int64(42), rootSpan.Events[0].Attributes["event.int"])
		assert.Equal(t, uint32(0), rootSpan.Events[0].DroppedAttributesCount)

		assert.Equal(t, "root-event-2", rootSpan.Events[1].Name)
		assert.Equal(t, "World", rootSpan.Events[1].Attributes["event.string2"])
		assert.Equal(t, int64(100), rootSpan.Events[1].Attributes["event.int2"])
		assert.Equal(t, uint32(1), rootSpan.Events[1].DroppedAttributesCount)

		// Verify root span links
		assert.Equal(t, "linked-trace-1", rootSpan.Links[0].TraceID)
		assert.Equal(t, "linked-span-1", rootSpan.Links[0].SpanID)
		assert.Equal(t, "state1", rootSpan.Links[0].TraceState)
		assert.Equal(t, "Link1", rootSpan.Links[0].Attributes["link.string"])
		assert.Equal(t, int64(123), rootSpan.Links[0].Attributes["link.int"])
		assert.Equal(t, uint32(0), rootSpan.Links[0].DroppedAttributesCount)

		// Verify child span events and links
		childSpan := trace.Spans[1]
		assert.Equal(t, "child-event", childSpan.Events[0].Name)
		assert.Equal(t, "Child Event", childSpan.Events[0].Attributes["child.event.string"])
		assert.Equal(t, int64(50), childSpan.Events[0].Attributes["child.event.int"])

		assert.Equal(t, "linked-trace-2", childSpan.Links[0].TraceID)
		assert.Equal(t, "linked-span-2", childSpan.Links[0].SpanID)
		assert.Equal(t, "state2", childSpan.Links[0].TraceState)
		assert.Equal(t, "Child Link", childSpan.Links[0].Attributes["child.link.string"])
		assert.Equal(t, int64(456), childSpan.Links[0].Attributes["child.link.int"])
		assert.Equal(t, uint32(1), childSpan.Links[0].DroppedAttributesCount)
	})

	t.Run("TraceResourceAndScope", func(t *testing.T) {
		trace, err := helper.store.GetTrace(helper.ctx, "test-trace")
		assert.NoError(t, err)

		// Verify resource and scope (should be consistent across all spans)
		span := trace.Spans[0] // Check first span
		assert.Equal(t, "test-service", span.Resource.Attributes["service.name"])
		assert.Equal(t, "1.0.0", span.Resource.Attributes["service.version"])
		assert.Equal(t, uint32(0), span.Resource.DroppedAttributesCount)
		assert.Equal(t, "test-scope", span.Scope.Name)
		assert.Equal(t, "v1.0.0", span.Scope.Version)
		assert.Empty(t, span.Scope.Attributes)
		assert.Equal(t, uint32(0), span.Scope.DroppedAttributesCount)
	})
} 

// createTestTrace creates a comprehensive test trace with multiple spans, events, and links.
func createTestTrace() []telemetry.SpanData {
	baseTime := time.Now().UnixNano()
	event1Time := baseTime + 100 * time.Millisecond.Nanoseconds()
	event2Time := baseTime + 200 * time.Millisecond.Nanoseconds()

	return []telemetry.SpanData{
		{
			// Root span with service name
			TraceID:      "test-trace",
			SpanID:       "root-span",
			ParentSpanID: "",
			Name:         "root-operation",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    baseTime,
			EndTime:      baseTime + time.Second.Nanoseconds(),
			Attributes: map[string]any{
				"root.string": "root-value",
				"root.int":    int64(42),
				"root.float":  float64(3.14),
				"root.bool":   true,
				"root.list":   []string{"one", "two", "three"},
			},
			Events: []telemetry.EventData{
				{
					Name:      "root-event-1",
					Timestamp: event1Time,
					Attributes: map[string]any{
						"event.string": "Hello",
						"event.int":    int64(42),
					},
					DroppedAttributesCount: 0,
				},
				{
					Name:      "root-event-2",
					Timestamp: event2Time,
					Attributes: map[string]any{
						"event.string2": "World",
						"event.int2":    int64(100),
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
						"link.string": "Link1",
						"link.int":    int64(123),
					},
					DroppedAttributesCount: 0,
				},
			},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name": "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_OK",
			StatusMessage: "",
		},
		{
			// Child span with error status
			TraceID:      "test-trace",
			SpanID:       "child-span",
			ParentSpanID: "root-span",
			Name:         "child-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 50 * time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 900 * time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"child.string": "child-value",
				"child.int":    int64(24),
				"child.float":  float64(2.71),
				"child.bool":   false,
				"child.list":   []int64{1, 2, 3, 4, 5},
			},
			Events: []telemetry.EventData{
				{
					Name:      "child-event",
					Timestamp: baseTime + 150 * time.Millisecond.Nanoseconds(),
					Attributes: map[string]any{
						"child.event.string": "Child Event",
						"child.event.int":    int64(50),
					},
					DroppedAttributesCount: 0,
				},
			},
			Links: []telemetry.LinkData{
				{
					TraceID:    "linked-trace-2",
					SpanID:     "linked-span-2",
					TraceState: "state2",
					Attributes: map[string]any{
						"child.link.string": "Child Link",
						"child.link.int":    int64(456),
					},
					DroppedAttributesCount: 1,
				},
			},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name": "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_ERROR",
			StatusMessage: "operation failed",
		},
		{
			// Orphaned span (has parent but parent doesn't exist)
			TraceID:      "test-trace",
			SpanID:       "orphaned-span",
			ParentSpanID: "non-existent-parent",
			Name:         "orphaned-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 100 * time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 800 * time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"orphaned.string": "orphaned-value",
			},
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
			StatusCode:    "STATUS_CODE_UNSET",
			StatusMessage: "",
		},
	}
}