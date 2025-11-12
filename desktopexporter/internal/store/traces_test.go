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

	t.Run("AttributeDiscovery", func(t *testing.T) {
		// Get available attributes from the test trace (use a wide time range to include all test data)
		// Test data uses time.Now().UnixNano(), so we need a range that includes current time
		now := time.Now().UnixNano()
		attributes, err := helper.Store.GetTraceAttributes(helper.Ctx, now-24*time.Hour.Nanoseconds(), now+24*time.Hour.Nanoseconds())
		assert.NoError(t, err, "failed to get available trace attributes")

		assert.NotEmpty(t, attributes, "should have discovered attributes")

		// Group attributes by scope for easier verification
		attributesByScope := make(map[string][]AttributeSuggestion)
		for _, attr := range attributes {
			attributesByScope[attr.AttributeScope] = append(attributesByScope[attr.AttributeScope], attr)
		}

		// Verify we have attributes from all expected scopes
		expectedScopes := []string{"resource", "span", "event", "link"}
		for _, scope := range expectedScopes {
			assert.Contains(t, attributesByScope, scope, "should have %s attributes", scope)
		}

		// Verify specific resource attributes
		resourceAttrs := attributesByScope["resource"]
		resourceNames := make([]string, len(resourceAttrs))
		for i, attr := range resourceAttrs {
			resourceNames[i] = attr.Name
		}
		assert.Contains(t, resourceNames, "service.name", "should have service.name resource attribute")
		assert.Contains(t, resourceNames, "service.version", "should have service.version resource attribute")

		// Verify specific span attributes
		spanAttrs := attributesByScope["span"]
		spanNames := make([]string, len(spanAttrs))
		for i, attr := range spanAttrs {
			spanNames[i] = attr.Name
		}
		assert.Contains(t, spanNames, "root.string", "should have root.string span attribute")
		assert.Contains(t, spanNames, "root.int", "should have root.int span attribute")
		assert.Contains(t, spanNames, "root.float", "should have root.float span attribute")
		assert.Contains(t, spanNames, "root.bool", "should have root.bool span attribute")
		assert.Contains(t, spanNames, "root.list", "should have root.list span attribute")

		// Verify specific event attributes
		eventAttrs := attributesByScope["event"]
		eventNames := make([]string, len(eventAttrs))
		for i, attr := range eventAttrs {
			eventNames[i] = attr.Name
		}
		assert.Contains(t, eventNames, "event.string", "should have event.string event attribute")
		assert.Contains(t, eventNames, "event.int", "should have event.int event attribute")
		assert.Contains(t, eventNames, "event.bool", "should have event.bool event attribute")
		assert.Contains(t, eventNames, "event.float", "should have event.float event attribute")

		// Verify specific link attributes
		linkAttrs := attributesByScope["link"]
		linkNames := make([]string, len(linkAttrs))
		for i, attr := range linkAttrs {
			linkNames[i] = attr.Name
		}
		assert.Contains(t, linkNames, "link.string", "should have link.string link attribute")
		assert.Contains(t, linkNames, "link.int", "should have link.int link attribute")
		assert.Contains(t, linkNames, "link.float", "should have link.float link attribute")
		assert.Contains(t, linkNames, "link.bool", "should have link.bool link attribute")

		// Verify attribute types are correctly detected (converted to frontend format)
		for _, attr := range attributes {
			switch attr.Name {
			case "service.name", "root.string", "event.string", "link.string":
				assert.Equal(t, "string", attr.Type, "string attributes should have 'string' type")
			case "root.int", "event.int", "link.int":
				assert.Equal(t, "int64", attr.Type, "int attributes should have 'int64' type")
			case "root.float", "event.float", "link.float":
				assert.Equal(t, "float64", attr.Type, "float attributes should have 'float64' type")
			case "root.bool", "event.bool", "link.bool":
				assert.Equal(t, "boolean", attr.Type, "bool attributes should have 'boolean' type")
			case "root.list", "event.list":
				assert.Equal(t, "string[]", attr.Type, "string list attributes should have 'string[]' type (converted from 'string_list')")
			}
		}
	})
}

// TestSearchTraces tests the SearchTraces functionality with various query types
func TestSearchTraces(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Add test trace
	spans := createTestTrace()
	err := helper.Store.AddSpans(helper.Ctx, spans)
	assert.NoError(t, err, "failed to add test trace")

	baseTime := time.Now().UnixNano()
	startTime := baseTime - 24*time.Hour.Nanoseconds()
	endTime := baseTime + 24*time.Hour.Nanoseconds()

	t.Run("GlobalSearch_ResourceAttribute", func(t *testing.T) {
		// Search for "test-service" in resource attributes via global search
		query := &QueryNode{
			ID:   "query-1",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "test-service",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with test-service")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_SpanAttribute", func(t *testing.T) {
		// Search for "root-value" in span attributes via global search
		query := &QueryNode{
			ID:   "query-2",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "root-value",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with root-value")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_EventField", func(t *testing.T) {
		// Search for "root-event" in event names via global search
		query := &QueryNode{
			ID:   "query-3",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "root-event",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with root-event")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_EventAttribute", func(t *testing.T) {
		// Search for "Hello" in event attributes via global search
		query := &QueryNode{
			ID:   "query-4",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "Hello",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with Hello in event attributes")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_LinkAttribute", func(t *testing.T) {
		// Search for "Link1" in link attributes via global search
		query := &QueryNode{
			ID:   "query-5",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "Link1",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with Link1 in link attributes")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		// Search for something that doesn't exist
		query := &QueryNode{
			ID:   "query-6",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "nonexistent-value-12345",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.Empty(t, summaries, "should not find any traces")
	})

	t.Run("GlobalSearch_WithFieldCondition", func(t *testing.T) {
		// Global search combined with field condition (AND)
		query := &QueryNode{
			ID:   "query-7",
			Type: "group",
			Group: &QueryGroup{
				LogicalOperator: "AND",
				Children: []QueryNode{
					{
						ID:   "query-7a",
						Type: "condition",
						Query: &Query{
							Field: &FieldDefinition{
								SearchScope: "global",
							},
							FieldOperator: "CONTAINS",
							Value:         "test-service",
						},
					},
					{
						ID:   "query-7b",
						Type: "condition",
						Query: &Query{
							Field: &FieldDefinition{
								Name:        "Name",
								SearchScope: "field",
							},
							FieldOperator: "=",
							Value:         "root-operation",
						},
					},
				},
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "combined search should not error")
		assert.NotEmpty(t, summaries, "should find trace matching both conditions")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_UnionValueExtraction", func(t *testing.T) {
		// Test that union type values are correctly extracted using CAST to VARCHAR
		// Search for "42" which exists as int64 in span attributes (root.int: 42)
		// This verifies that CAST(unnest.value AS VARCHAR) correctly extracts
		// the value from the union type structure {tag: "int64", value: 42}
		query := &QueryNode{
			ID:   "query-8",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "42",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with 42 in attributes (union value extraction)")
		assert.Equal(t, "test-trace", summaries[0].TraceID)
	})

	t.Run("GlobalSearch_PumpkinPie", func(t *testing.T) {
		// Add a trace with "pumpkin.pie" as service.name to test union value extraction
		// Note: trace ID, span ID, and name do NOT contain "pumpkin" to ensure
		// the search only matches on attribute values
		pumpkinTrace := []traces.SpanData{
			{
				TraceID:      "test-trace-2",
				SpanID:       "test-span-2",
				ParentSpanID: "",
				Name:         "test-operation",
				Kind:         "SPAN_KIND_SERVER",
				StartTime:    baseTime,
				EndTime:      baseTime + time.Second.Nanoseconds(),
				Attributes:   map[string]any{},
				Events:       []traces.EventData{},
				Links:        []traces.LinkData{},
				Resource: &resource.ResourceData{
					Attributes: map[string]any{
						"service.name": "pumpkin.pie",
					},
					DroppedAttributesCount: 0,
				},
				Scope: &scope.ScopeData{
					Name:                   "test-scope",
					Version:                "v1.0.0",
					Attributes:             map[string]any{},
					DroppedAttributesCount: 0,
				},
				DroppedAttributesCount: 0,
				DroppedEventsCount:     0,
				DroppedLinksCount:      0,
				StatusCode:             "",
				StatusMessage:          "",
			},
		}
		err := helper.Store.AddSpans(helper.Ctx, pumpkinTrace)
		assert.NoError(t, err, "failed to add pumpkin trace")

		// Search for "pumpkin" - should find "pumpkin.pie" in resource attributes
		// This verifies that CAST(unnest.value AS VARCHAR) correctly extracts
		// union type values for searching
		query := &QueryNode{
			ID:   "query-9",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					SearchScope: "global",
				},
				FieldOperator: "CONTAINS",
				Value:         "pumpkin",
			},
		}

		summaries, err := helper.Store.SearchTraces(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err, "global search should not error")
		assert.NotEmpty(t, summaries, "should find trace with pumpkin.pie")
		// Should find test-trace-2 which has pumpkin.pie in resource attributes
		foundPumpkin := false
		for _, summary := range summaries {
			if summary.TraceID == "test-trace-2" {
				foundPumpkin = true
				break
			}
		}
		assert.True(t, foundPumpkin, "should find test-trace-2 with pumpkin.pie in resource attributes")
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
