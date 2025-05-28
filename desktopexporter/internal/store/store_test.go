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

// TestAttributeTypes tests basic attribute type preservation using native Go types.
// This test verifies that when attributes are provided as their native Go types
// (string, int64, float64, bool, []string, etc.), they are correctly stored in
// DuckDB using the appropriate union tags and retrieved with their original types intact.
// This is the "happy path" where types are already correctly typed from the source.
func TestAttributeTypes(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	// Create a test span with all supported attribute types using native Go types
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
				// Native Go types - should map directly to DuckDB union types
				"string1":    "Koala",
				"string2":    "Bear",
				"bigint1":    int64(42),
				"double":     float64(3.14),
				"boolean":    true,
				"str_list":   []string{"one", "two", "three"},
				"int_list":   []int64{1, 2, 3},
				"float_list": []float64{1.1, 2.2, 3.3},
				"bool_list":  []bool{true, false, true},

				// Test different integer types (should all become bigint)
				"int32_val":  int32(100),
				"uint32_val": uint32(200),

				// Test different float types (should all become double)
				"float32_val": float32(2.5),
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

	// Verify arrays: they come back as []any but each element should be castable to the correct type
	strList := span.Attributes["str_list"].([]any)
	assert.Len(t, strList, 3)
	for _, item := range strList {
		assert.IsType(t, "", item, "str_list elements should be strings")
	}
	assert.Equal(t, "one", strList[0])
	assert.Equal(t, "two", strList[1])
	assert.Equal(t, "three", strList[2])

	intList := span.Attributes["int_list"].([]any)
	assert.Len(t, intList, 3)
	for _, item := range intList {
		_, ok := item.(int64)
		assert.True(t, ok, "int_list elements should be castable to int64")
	}
	assert.Equal(t, int64(1), intList[0])
	assert.Equal(t, int64(2), intList[1])
	assert.Equal(t, int64(3), intList[2])

	floatList := span.Attributes["float_list"].([]any)
	assert.Len(t, floatList, 3)
	for _, item := range floatList {
		_, ok := item.(float64)
		assert.True(t, ok, "float_list elements should be castable to float64")
	}
	assert.Equal(t, float64(1.1), floatList[0])
	assert.Equal(t, float64(2.2), floatList[1])
	assert.Equal(t, float64(3.3), floatList[2])

	boolList := span.Attributes["bool_list"].([]any)
	assert.Len(t, boolList, 3)
	for _, item := range boolList {
		_, ok := item.(bool)
		assert.True(t, ok, "bool_list elements should be castable to bool")
	}
	assert.Equal(t, true, boolList[0])
	assert.Equal(t, false, boolList[1])
	assert.Equal(t, true, boolList[2])

	// Verify different numeric types are converted to DuckDB union-supported types
	// int32 -> BIGINT (int64), float32 -> DOUBLE (float64)
	assert.Equal(t, int64(100), span.Attributes["int32_val"])     // int32 becomes int64
	assert.Equal(t, int64(200), span.Attributes["uint32_val"])    // uint32 becomes int64
	assert.Equal(t, float64(2.5), span.Attributes["float32_val"]) // float32 becomes float64
}

// TestArrayAttributeTypeCasting tests how we handle []any array attributes, which is how
// OpenTelemetry Go implements array attributes in practice. The test verifies that:
//
//  1. Homogeneous arrays: When all elements in a []any can be cast to the same primitive type,
//     we detect that type and create the appropriate typed list (str_list, bigint_list, etc.)
//
//  2. Mixed compatible types: When a []any contains different but compatible numeric types
//     (e.g., int, int32, int64), we convert them all to the target type (int64 for integers,
//     float64 for floats) to create a homogeneous typed list.
//
//  3. Fallback behavior: When a []any contains truly mixed incompatible types, we fall back
//     to converting everything to strings and create a str_list.
//
// 4. Edge cases: Empty arrays, single-element arrays, and other boundary conditions.
//
// This approach preserves type information where possible while gracefully handling the
// reality that OpenTelemetry attributes come as []any from the instrumentation libraries.
// The goal is to store data in DuckDB's strongly-typed UNION format while maintaining
// compatibility with the loosely-typed nature of OpenTelemetry attribute values.
func TestArrayAttributeTypeCasting(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	// Create a test span with []any array attributes that should be castable to primitive types
	baseTime := time.Now()
	spans := []telemetry.SpanData{
		{
			TraceID:      "test-trace-arrays",
			SpanID:       "test-span-arrays",
			ParentSpanID: "",
			Name:         "test-span-arrays",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    baseTime,
			EndTime:      baseTime.Add(time.Second),
			Attributes: map[string]any{
				// []any arrays where all elements are the same type (like OpenTelemetry Go)
				"any_string_list": []any{"apple", "banana", "cherry"},
				"any_int_list":    []any{int64(10), int64(20), int64(30)},
				"any_float_list":  []any{float64(1.5), float64(2.5), float64(3.5)},
				"any_bool_list":   []any{true, false, true},

				// Mixed integer types in []any (should all become bigint_list)
				"any_mixed_ints": []any{int(1), int32(2), int64(3), uint(4), uint32(5)},

				// Mixed float types in []any (should all become double_list)
				"any_mixed_floats": []any{float32(1.1), float64(2.2), float32(3.3)},

				// Edge cases
				"any_empty_list":  []any{},
				"any_single_item": []any{"single"},

				// Additional empty list tests for different types
				"empty_string_list": []string{},
				"empty_int_list":    []int64{},
				"empty_float_list":  []float64{},
				"empty_bool_list":   []bool{},

				// Mixed types that should fall back to string list
				"any_mixed_types": []any{"string", int64(42), float64(3.14)},

				// uint64 overflow test - large uint64 should fall back to string
				"any_uint64_overflow": []any{uint64(18446744073709551615), uint64(100)}, // MaxUint64 and small value

				// Native []uint64 with overflow - should convert to []string
				"native_uint64_overflow": []uint64{uint64(18446744073709551615), uint64(100)}, // MaxUint64 and small value
				"native_uint64_safe":     []uint64{uint64(100), uint64(200), uint64(300)},     // All safe values

				// More comprehensive mixed type tests
				"any_mixed_complex":       []any{"text", int64(123), float64(4.56), true, "more text"},
				"any_string_and_bool":     []any{"hello", true, "world", false},
				"any_numbers_and_strings": []any{int64(1), "two", float64(3.0), "four"},
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
	trace, err := store.GetTrace(ctx, "test-trace-arrays")
	assert.NoError(t, err, "could not get trace")
	assert.Len(t, trace.Spans, 1, "expected 1 span")

	span := trace.Spans[0]

	// Arrays come back as []any - verify each element can be cast to correct type

	// Homogeneous arrays
	strList := span.Attributes["any_string_list"].([]any)
	assert.Len(t, strList, 3)
	for _, item := range strList {
		assert.IsType(t, "", item)
	}
	assert.Equal(t, "apple", strList[0])
	assert.Equal(t, "banana", strList[1])
	assert.Equal(t, "cherry", strList[2])

	intList := span.Attributes["any_int_list"].([]any)
	assert.Len(t, intList, 3)
	for _, item := range intList {
		_, ok := item.(int64)
		assert.True(t, ok, "should be int64")
	}
	assert.Equal(t, int64(10), intList[0])
	assert.Equal(t, int64(20), intList[1])
	assert.Equal(t, int64(30), intList[2])

	floatList := span.Attributes["any_float_list"].([]any)
	assert.Len(t, floatList, 3)
	for _, item := range floatList {
		_, ok := item.(float64)
		assert.True(t, ok, "should be float64")
	}
	assert.Equal(t, float64(1.5), floatList[0])
	assert.Equal(t, float64(2.5), floatList[1])
	assert.Equal(t, float64(3.5), floatList[2])

	boolList := span.Attributes["any_bool_list"].([]any)
	assert.Len(t, boolList, 3)
	for _, item := range boolList {
		_, ok := item.(bool)
		assert.True(t, ok, "should be bool")
	}
	assert.Equal(t, true, boolList[0])
	assert.Equal(t, false, boolList[1])
	assert.Equal(t, true, boolList[2])

	// Mixed compatible types -> converted to DuckDB union types
	mixedIntList := span.Attributes["any_mixed_ints"].([]any)
	assert.Len(t, mixedIntList, 5)
	for _, item := range mixedIntList {
		_, ok := item.(int64)
		assert.True(t, ok, "mixed ints should become int64")
	}
	assert.Equal(t, int64(1), mixedIntList[0])
	assert.Equal(t, int64(2), mixedIntList[1])
	assert.Equal(t, int64(3), mixedIntList[2])
	assert.Equal(t, int64(4), mixedIntList[3])
	assert.Equal(t, int64(5), mixedIntList[4])

	mixedFloatList := span.Attributes["any_mixed_floats"].([]any)
	assert.Len(t, mixedFloatList, 3)
	for _, item := range mixedFloatList {
		_, ok := item.(float64)
		assert.True(t, ok, "mixed floats should become float64")
	}
	assert.Equal(t, float64(float32(1.1)), mixedFloatList[0])
	assert.Equal(t, float64(2.2), mixedFloatList[1])
	assert.Equal(t, float64(float32(3.3)), mixedFloatList[2])

	// Edge cases
	emptyList := span.Attributes["any_empty_list"].([]any)
	assert.Len(t, emptyList, 0)

	singleList := span.Attributes["any_single_item"].([]any)
	assert.Len(t, singleList, 1)
	assert.Equal(t, "single", singleList[0])

	// Additional empty list tests - all should come back as empty []any
	emptyStringList := span.Attributes["empty_string_list"].([]any)
	assert.Len(t, emptyStringList, 0)
	assert.Equal(t, []any{}, emptyStringList)

	emptyIntList := span.Attributes["empty_int_list"].([]any)
	assert.Len(t, emptyIntList, 0)
	assert.Equal(t, []any{}, emptyIntList)

	emptyFloatList := span.Attributes["empty_float_list"].([]any)
	assert.Len(t, emptyFloatList, 0)
	assert.Equal(t, []any{}, emptyFloatList)

	emptyBoolList := span.Attributes["empty_bool_list"].([]any)
	assert.Len(t, emptyBoolList, 0)
	assert.Equal(t, []any{}, emptyBoolList)

	// Mixed incompatible types -> fallback to string
	mixedList := span.Attributes["any_mixed_types"].([]any)
	assert.Len(t, mixedList, 3)
	for _, item := range mixedList {
		assert.IsType(t, "", item, "mixed types should become strings")
	}
	assert.Equal(t, "string", mixedList[0])
	assert.Equal(t, "42", mixedList[1])
	assert.Equal(t, "3.14", mixedList[2])

	// uint64 overflow should fall back to string
	overflowList := span.Attributes["any_uint64_overflow"].([]any)
	assert.Len(t, overflowList, 2)
	for _, item := range overflowList {
		assert.IsType(t, "", item, "uint64 overflow should become strings")
	}
	assert.Equal(t, "18446744073709551615", overflowList[0]) // MaxUint64 as string
	assert.Equal(t, "100", overflowList[1])

	// Native []uint64 with overflow should also fall back to string
	nativeOverflowList := span.Attributes["native_uint64_overflow"].([]any)
	assert.Len(t, nativeOverflowList, 2)
	for _, item := range nativeOverflowList {
		assert.IsType(t, "", item, "native []uint64 overflow should become strings")
	}
	assert.Equal(t, "18446744073709551615", nativeOverflowList[0]) // MaxUint64 as string
	assert.Equal(t, "100", nativeOverflowList[1])

	// Native []uint64 with safe values should become bigint_list
	nativeSafeList := span.Attributes["native_uint64_safe"].([]any)
	assert.Len(t, nativeSafeList, 3)
	for _, item := range nativeSafeList {
		_, ok := item.(int64)
		assert.True(t, ok, "safe uint64 values should become int64")
	}
	assert.Equal(t, int64(100), nativeSafeList[0])
	assert.Equal(t, int64(200), nativeSafeList[1])
	assert.Equal(t, int64(300), nativeSafeList[2])

	complexMixedList := span.Attributes["any_mixed_complex"].([]any)
	assert.Len(t, complexMixedList, 5)
	for _, item := range complexMixedList {
		assert.IsType(t, "", item, "complex mixed should become strings")
	}
	assert.Equal(t, "text", complexMixedList[0])
	assert.Equal(t, "123", complexMixedList[1])
	assert.Equal(t, "4.56", complexMixedList[2])
	assert.Equal(t, "true", complexMixedList[3])
	assert.Equal(t, "more text", complexMixedList[4])

	stringBoolList := span.Attributes["any_string_and_bool"].([]any)
	assert.Len(t, stringBoolList, 4)
	for _, item := range stringBoolList {
		assert.IsType(t, "", item, "string+bool should become strings")
	}
	assert.Equal(t, "hello", stringBoolList[0])
	assert.Equal(t, "true", stringBoolList[1])
	assert.Equal(t, "world", stringBoolList[2])
	assert.Equal(t, "false", stringBoolList[3])

	numbersStringsList := span.Attributes["any_numbers_and_strings"].([]any)
	assert.Len(t, numbersStringsList, 4)
	for _, item := range numbersStringsList {
		assert.IsType(t, "", item, "numbers+strings should become strings")
	}
	assert.Equal(t, "1", numbersStringsList[0])
	assert.Equal(t, "two", numbersStringsList[1])
	assert.Equal(t, "3", numbersStringsList[2])
	assert.Equal(t, "four", numbersStringsList[3])
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
	assert.True(t, event1Time.Equal(event1.Timestamp), "event1 timestamp mismatch: expected %v, got %v", event1Time, event1.Timestamp)
	assert.Equal(t, "Hello", event1.Attributes["event_string"])
	assert.Equal(t, int64(42), event1.Attributes["event_int"])
	assert.Equal(t, float64(3.14), event1.Attributes["event_float"])
	assert.Equal(t, true, event1.Attributes["event_bool"])

	// Check event_list as []any and verify individual elements
	eventList := event1.Attributes["event_list"].([]any)
	assert.Len(t, eventList, 3)
	assert.Equal(t, "one", eventList[0])
	assert.Equal(t, "two", eventList[1])
	assert.Equal(t, "three", eventList[2])

	assert.Equal(t, uint32(0), event1.DroppedAttributesCount)

	// Check second event
	event2 := span.Events[1]
	assert.Equal(t, "event2", event2.Name)
	assert.True(t, event2Time.Equal(event2.Timestamp), "event2 timestamp mismatch: expected %v, got %v", event2Time, event2.Timestamp)
	assert.Equal(t, "World", event2.Attributes["event_string2"])
	assert.Equal(t, int64(100), event2.Attributes["event_int2"])
	assert.Equal(t, float64(6.28), event2.Attributes["event_float2"])
	assert.Equal(t, false, event2.Attributes["event_bool2"])

	// Check event_list2 as []any and verify individual elements
	eventList2 := event2.Attributes["event_list2"].([]any)
	assert.Len(t, eventList2, 5)
	assert.Equal(t, int64(1), eventList2[0])
	assert.Equal(t, int64(2), eventList2[1])
	assert.Equal(t, int64(3), eventList2[2])
	assert.Equal(t, int64(4), eventList2[3])
	assert.Equal(t, int64(5), eventList2[4])

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

	// Check link_list as []any and verify individual elements
	linkList := link1.Attributes["link_list"].([]any)
	assert.Len(t, linkList, 3)
	assert.Equal(t, float64(1.1), linkList[0])
	assert.Equal(t, float64(2.2), linkList[1])
	assert.Equal(t, float64(3.3), linkList[2])

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

	// Check link_list2 as []any and verify individual elements
	linkList2 := link2.Attributes["link_list2"].([]any)
	assert.Len(t, linkList2, 3)
	assert.Equal(t, true, linkList2[0])
	assert.Equal(t, false, linkList2[1])
	assert.Equal(t, true, linkList2[2])

	assert.Equal(t, uint32(2), link2.DroppedAttributesCount)
}

func TestResourceAndScopeAttributes(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	defer store.Close()

	baseTime := time.Now()
	spans := []telemetry.SpanData{
		{
			TraceID:      "test-trace-resource-scope",
			SpanID:       "test-span-1",
			ParentSpanID: "",
			Name:         "test-span",
			Kind:         "SPAN_KIND_SERVER",
			StartTime:    baseTime,
			EndTime:      baseTime.Add(time.Second),
			Attributes: map[string]any{
				"span_attr": "span_value",
			},
			Events: []telemetry.EventData{},
			Links:  []telemetry.LinkData{},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "v1.0.0",
					"resource_int":    int64(42),
					"resource_bool":   true,
					"resource_list":   []string{"res1", "res2"},
					"empty_list":      []string{}, // Test empty list
				},
				DroppedAttributesCount: 1,
			},
			Scope: &telemetry.ScopeData{
				Name:    "test-scope",
				Version: "v2.0.0",
				Attributes: map[string]any{
					"scope_string": "scope_value",
					"scope_float":  float64(3.14),
					"scope_array":  []int64{10, 20, 30},
					"empty_array":  []int64{}, // Test empty list
				},
				DroppedAttributesCount: 2,
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
	trace, err := store.GetTrace(ctx, "test-trace-resource-scope")
	assert.NoError(t, err, "could not get trace")
	assert.Len(t, trace.Spans, 1, "expected 1 span")

	span := trace.Spans[0]

	// Verify span attributes
	assert.Equal(t, "span_value", span.Attributes["span_attr"])

	// Verify resource attributes
	assert.NotNil(t, span.Resource, "resource should not be nil")
	assert.Equal(t, "test-service", span.Resource.Attributes["service.name"])
	assert.Equal(t, "v1.0.0", span.Resource.Attributes["service.version"])
	assert.Equal(t, int64(42), span.Resource.Attributes["resource_int"])
	assert.Equal(t, true, span.Resource.Attributes["resource_bool"])
	assert.Equal(t, uint32(1), span.Resource.DroppedAttributesCount)

	// Check resource list as []any
	resourceList := span.Resource.Attributes["resource_list"].([]any)
	assert.Len(t, resourceList, 2)
	assert.Equal(t, "res1", resourceList[0])
	assert.Equal(t, "res2", resourceList[1])

	// Check empty list in resource - should be empty []any
	emptyList := span.Resource.Attributes["empty_list"].([]any)
	assert.Len(t, emptyList, 0)
	assert.Equal(t, []any{}, emptyList)

	// Verify scope attributes
	assert.NotNil(t, span.Scope, "scope should not be nil")
	assert.Equal(t, "test-scope", span.Scope.Name)
	assert.Equal(t, "v2.0.0", span.Scope.Version)
	assert.Equal(t, "scope_value", span.Scope.Attributes["scope_string"])
	assert.Equal(t, float64(3.14), span.Scope.Attributes["scope_float"])
	assert.Equal(t, uint32(2), span.Scope.DroppedAttributesCount)

	// Check scope array as []any
	scopeArray := span.Scope.Attributes["scope_array"].([]any)
	assert.Len(t, scopeArray, 3)
	assert.Equal(t, int64(10), scopeArray[0])
	assert.Equal(t, int64(20), scopeArray[1])
	assert.Equal(t, int64(30), scopeArray[2])

	// Check empty array in scope - should be empty []any
	emptyArray := span.Scope.Attributes["empty_array"].([]any)
	assert.Len(t, emptyArray, 0)
	assert.Equal(t, []any{}, emptyArray)
}
