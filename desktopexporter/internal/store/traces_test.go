package store

import (
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
	"github.com/stretchr/testify/assert"
)

// TestTraceSummaryOrdering verifies that trace summaries are ordered by start time (newest first).
func TestTraceSummaryOrdering(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()

	// Create test spans with different timing scenarios
	spans := []traces.SpanData{
		{
			// Trace 1: Middle time
			TraceID:      "trace1",
			SpanID:       "span1",
			ParentSpanID: "", // root span
			Name:         "root middle",
			StartTime:    baseTime + time.Second.Nanoseconds(), // t+1
			EndTime:      baseTime + 2*time.Second.Nanoseconds(),
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name": "service1",
				},
			},
			Scope: &scope.ScopeData{},
		},
		{
			// Trace 2: Oldest time
			TraceID:      "trace2",
			SpanID:       "span2",
			ParentSpanID: "missing_parent",
			Name:         "earliest no root",
			StartTime:    baseTime, // t+0
			EndTime:      baseTime + 2*time.Second.Nanoseconds(),
			Resource: &resource.ResourceData{
				Attributes: map[string]any{},
			},
			Scope: &scope.ScopeData{},
		},
		{
			// Trace 3: Newest time
			TraceID:      "trace3",
			SpanID:       "span3",
			ParentSpanID: "", // root span
			Name:         "root last",
			StartTime:    baseTime + 2*time.Second.Nanoseconds(), // t+2
			EndTime:      baseTime + 3*time.Second.Nanoseconds(),
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name": "service3",
				},
			},
			Scope: &scope.ScopeData{},
		},
	}

	// Add spans to store
	err := helper.Store.AddSpans(helper.Ctx, spans)
	assert.NoError(t, err, "failed to add spans")

	// Get summaries
	summaries, err := helper.Store.GetTraceSummaries(helper.Ctx)
	assert.NoError(t, err, "failed to get trace summaries")

	// Should have all three traces
	assert.Len(t, summaries, 3, "expected 3 traces")

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
	helper, teardown := SetupTest(t)
	defer teardown()

	_, err := helper.Store.GetTrace(helper.Ctx, "non-existent-trace")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTraceIDNotFound)
}

// TestEmptySpans verifies handling of empty span lists and empty stores.
func TestEmptySpans(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Test adding empty span list
	err := helper.Store.AddSpans(helper.Ctx, []traces.SpanData{})
	assert.NoError(t, err)

	// Test getting summaries from empty store
	summaries, err := helper.Store.GetTraceSummaries(helper.Ctx)
	assert.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestClearTraces verifies that all traces can be cleared from the store.
func TestClearTraces(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Add test trace
	spans := createTestTrace()
	err := helper.Store.AddSpans(helper.Ctx, spans)
	assert.NoError(t, err)

	// Verify trace exists
	summaries, err := helper.Store.GetTraceSummaries(helper.Ctx)
	assert.NoError(t, err)
	assert.Len(t, summaries, 1)

	// Clear traces
	err = helper.Store.ClearTraces(helper.Ctx)
	assert.NoError(t, err)

	// Verify store is empty
	summaries, err = helper.Store.GetTraceSummaries(helper.Ctx)
	assert.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestTraceSuite runs a comprehensive suite of tests on a single trace.
func TestTraceSuite(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Add the test trace
	spans := createTestTrace()
	err := helper.Store.AddSpans(helper.Ctx, spans)
	assert.NoError(t, err, "failed to add test trace")

	// Test the trace hierarchical functionality
	t.Run("TraceHierarchicalStructure", func(t *testing.T) {
		// Get the trace
		trace, err := helper.Store.GetTrace(helper.Ctx, "test-trace")
		assert.NoError(t, err, "failed to get trace")
		assert.NotEmpty(t, trace.Spans)

		// Basic validation that we have the expected spans
		assert.Equal(t, "test-trace", trace.TraceID)
		assert.Equal(t, "test-trace", trace.Spans[0].SpanData.TraceID)
		assert.Equal(t, "root-span", trace.Spans[0].SpanData.SpanID)
		assert.Len(t, trace.Spans, 9) // Should have 9 spans

		// Validate depth-first order: root first, then its earliest child, then that child's earliest child, etc.
		// Expected order: root-span -> child-span -> grandchild-span -> great-grandchild-span -> child-span-2 -> child2-child-span -> orphaned-span -> orphaned-child-span -> orphaned-grandchild-span
		assert.Equal(t, "root-span", trace.Spans[0].SpanData.SpanID)
		assert.Equal(t, "child-span", trace.Spans[1].SpanData.SpanID)               // root's earliest child
		assert.Equal(t, "grandchild-span", trace.Spans[2].SpanData.SpanID)          // child-span's earliest child
		assert.Equal(t, "great-grandchild-span", trace.Spans[3].SpanData.SpanID)    // grandchild-span's child
		assert.Equal(t, "child-span-2", trace.Spans[4].SpanData.SpanID)             // root's later child
		assert.Equal(t, "child2-child-span", trace.Spans[5].SpanData.SpanID)        // child-span-2's child
		assert.Equal(t, "orphaned-span", trace.Spans[6].SpanData.SpanID)            // orphan span
		assert.Equal(t, "orphaned-child-span", trace.Spans[7].SpanData.SpanID)      // orphaned-span's child
		assert.Equal(t, "orphaned-grandchild-span", trace.Spans[8].SpanData.SpanID) // orphaned-child-span's child
	})

	t.Run("TraceSummary", func(t *testing.T) {
		summaries, err := helper.Store.GetTraceSummaries(helper.Ctx)
		assert.NoError(t, err)
		assert.Len(t, summaries, 1, "should have one trace summary")

		summary := summaries[0]
		assert.Equal(t, "test-trace", summary.TraceID)
		assert.Equal(t, uint32(9), summary.SpanCount)
		assert.NotNil(t, summary.RootSpan)
		assert.Equal(t, "test-service", summary.RootSpan.ServiceName)
		assert.Equal(t, "root-operation", summary.RootSpan.Name)
	})

	t.Run("TraceNotFound", func(t *testing.T) {
		_, err := helper.Store.GetTrace(helper.Ctx, "non-existent-trace")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrTraceIDNotFound)
	})
}

// createTestTrace creates a comprehensive test trace with multiple spans, events, and links.
func createTestTrace() []traces.SpanData {
	baseTime := time.Now().UnixNano()
	event1Time := baseTime + 100*time.Millisecond.Nanoseconds()
	event2Time := baseTime + 200*time.Millisecond.Nanoseconds()

	return []traces.SpanData{
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
			Events: []traces.EventData{
				{
					Name:      "root-event-1",
					Timestamp: event1Time,
					Attributes: map[string]any{
						"event.string": "Hello",
						"event.int":    int64(42),
						"event.bool":   true,
						"event.float":  float64(3.14),
					},
					DroppedAttributesCount: 0,
				},
				{
					Name:      "root-event-2",
					Timestamp: event2Time,
					Attributes: map[string]any{
						"event.string2": "World",
						"event.int2":    int64(100),
						"event.list":    []string{"a", "b", "c"},
					},
					DroppedAttributesCount: 1,
				},
			},
			Links: []traces.LinkData{
				{
					TraceID:    "linked-trace-1",
					SpanID:     "linked-span-1",
					TraceState: "state1",
					Attributes: map[string]any{
						"link.string": "Link1",
						"link.int":    int64(123),
						"link.float":  float64(2.71),
						"link.bool":   false,
					},
					DroppedAttributesCount: 0,
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
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
			StartTime:    baseTime + 50*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 900*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"child.string": "child-value",
				"child.int":    int64(24),
				"child.float":  float64(2.71),
				"child.bool":   false,
				"child.list":   []int64{1, 2, 3, 4, 5},
			},
			Events: []traces.EventData{
				{
					Name:      "child-event",
					Timestamp: baseTime + 150*time.Millisecond.Nanoseconds(),
					Attributes: map[string]any{
						"child.event.string": "Child Event",
						"child.event.int":    int64(50),
						"child.event.bool":   false,
						"child.event.float":  float64(1.618),
					},
					DroppedAttributesCount: 0,
				},
			},
			Links: []traces.LinkData{
				{
					TraceID:    "linked-trace-2",
					SpanID:     "linked-span-2",
					TraceState: "state2",
					Attributes: map[string]any{
						"child.link.string": "Child Link",
						"child.link.int":    int64(456),
						"child.link.float":  float64(1.414),
						"child.link.bool":   true,
						"child.link.list":   []int64{1, 2, 3, 4, 5},
					},
					DroppedAttributesCount: 1,
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_ERROR",
			StatusMessage: "operation failed",
		},
		{
			// Second child of root span
			TraceID:      "test-trace",
			SpanID:       "child-span-2",
			ParentSpanID: "root-span",
			Name:         "child-operation-2",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 75*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 850*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"child2.string": "child2-value",
				"child2.int":    int64(99),
				"child2.float":  float64(1.414),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_OK",
			StatusMessage: "",
		},
		{
			// Grandchild span (child of child-span)
			TraceID:      "test-trace",
			SpanID:       "grandchild-span",
			ParentSpanID: "child-span",
			Name:         "grandchild-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 200*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 700*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"grandchild.string": "grandchild-value",
				"grandchild.int":    int64(123),
				"grandchild.float":  float64(2.236),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_OK",
			StatusMessage: "",
		},
		{
			// Great-grandchild span (child of grandchild-span)
			TraceID:      "test-trace",
			SpanID:       "great-grandchild-span",
			ParentSpanID: "grandchild-span",
			Name:         "great-grandchild-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 250*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 600*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"great-grandchild.string": "great-grandchild-value",
				"great-grandchild.int":    int64(456),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_ERROR",
			StatusMessage: "deep operation failed",
		},
		{
			// Child of child-span-2
			TraceID:      "test-trace",
			SpanID:       "child2-child-span",
			ParentSpanID: "child-span-2",
			Name:         "child2-child-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 150*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 750*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"child2-child.string": "child2-child-value",
				"child2-child.int":    int64(789),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_OK",
			StatusMessage: "",
		},
		{
			// Orphaned span (has parent but parent doesn't exist)
			TraceID:      "test-trace",
			SpanID:       "orphaned-span",
			ParentSpanID: "non-existent-parent",
			Name:         "orphaned-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 100*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 800*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"orphaned.string": "orphaned-value",
			},
			Resource: &resource.ResourceData{
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_UNSET",
			StatusMessage: "",
		},
		{
			// Child of orphaned span
			TraceID:      "test-trace",
			SpanID:       "orphaned-child-span",
			ParentSpanID: "orphaned-span",
			Name:         "orphaned-child-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 120*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 750*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"orphaned-child.string": "orphaned-child-value",
				"orphaned-child.int":    int64(555),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_OK",
			StatusMessage: "",
		},
		{
			// Grandchild of orphaned span
			TraceID:      "test-trace",
			SpanID:       "orphaned-grandchild-span",
			ParentSpanID: "orphaned-child-span",
			Name:         "orphaned-grandchild-operation",
			Kind:         "SPAN_KIND_INTERNAL",
			StartTime:    baseTime + 140*time.Millisecond.Nanoseconds(),
			EndTime:      baseTime + 700*time.Millisecond.Nanoseconds(),
			Attributes: map[string]any{
				"orphaned-grandchild.string": "orphaned-grandchild-value",
				"orphaned-grandchild.int":    int64(777),
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			StatusCode:    "STATUS_CODE_ERROR",
			StatusMessage: "orphaned operation failed",
		},
	}
}
