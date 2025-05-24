package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestMarshalEvents(t *testing.T) {
	// Create test events with various attribute types
	baseTime := time.Unix(0, 0)
	events := []telemetry.EventData{
		{
			Name:      "event1",
			Timestamp: baseTime.Add(100 * time.Millisecond),
			Attributes: map[string]any{
				"string1":      "Hello",
				"int1":         int64(42),
				"float1":       float64(3.14),
				"bool1":        true,
				"str_list":     []string{"one", "two", "three"},
				"int_list":     []int64{1, 2, 3},
				"float_list":   []float64{1.1, 2.2, 3.3},
				"bool_list":    []bool{true, false, true},
			},
			DroppedAttributesCount: 0,
		},
		{
			Name:      "event2",
			Timestamp: baseTime.Add(200 * time.Millisecond),
			Attributes: map[string]any{
				"string2":      "World",
				"int2":         int64(100),
				"float2":       float64(6.28),
				"bool2":        false,
				"str_list2":    []string{"a", "b", "c"},
				"int_list2":    []int64{10, 20, 30},
				"float_list2":  []float64{10.1, 20.2, 30.3},
				"bool_list2":   []bool{false, true, false},
			},
			DroppedAttributesCount: 1,
		},
	}

	// Marshal events
	jsonStr := MarshalEvents(events)
	
	// Verify it's valid JSON
	var unmarshaled []map[string]any
	err := json.Unmarshal([]byte(jsonStr), &unmarshaled)
	assert.NoError(t, err, "should be valid JSON")
	assert.Len(t, unmarshaled, 2, "should have 2 events")

	// Check first event
	event1 := unmarshaled[0]
	assert.Equal(t, "event1", event1["name"])
	assert.Equal(t, baseTime.Add(100*time.Millisecond).Format(time.RFC3339Nano), event1["timestamp"])
	assert.Equal(t, float64(0), event1["droppedAttributesCount"])

	// Check attributes of first event
	attrs1 := event1["attributes"].(map[string]any)
	assert.Equal(t, "Hello", attrs1["string1"].(map[string]any)["str"])
	assert.Equal(t, float64(42), attrs1["int1"].(map[string]any)["bigint"])
	assert.Equal(t, 3.14, attrs1["float1"].(map[string]any)["double"])
	assert.Equal(t, true, attrs1["bool1"].(map[string]any)["boolean"])
	
	// Check lists
	strList1 := attrs1["str_list"].(map[string]any)["str_list"].([]any)
	assert.Equal(t, "one", strList1[0])
	assert.Equal(t, "two", strList1[1])
	assert.Equal(t, "three", strList1[2])
	
	intList1 := attrs1["int_list"].(map[string]any)["bigint_list"].([]any)
	assert.Equal(t, float64(1), intList1[0])
	assert.Equal(t, float64(2), intList1[1])
	assert.Equal(t, float64(3), intList1[2])
	
	floatList1 := attrs1["float_list"].(map[string]any)["double_list"].([]any)
	assert.Equal(t, 1.1, floatList1[0])
	assert.Equal(t, 2.2, floatList1[1])
	assert.Equal(t, 3.3, floatList1[2])
	
	boolList1 := attrs1["bool_list"].(map[string]any)["boolean_list"].([]any)
	assert.Equal(t, true, boolList1[0])
	assert.Equal(t, false, boolList1[1])
	assert.Equal(t, true, boolList1[2])

	// Check second event
	event2 := unmarshaled[1]
	assert.Equal(t, "event2", event2["name"])
	assert.Equal(t, baseTime.Add(200*time.Millisecond).Format(time.RFC3339Nano), event2["timestamp"])
	assert.Equal(t, float64(1), event2["droppedAttributesCount"])
}

func TestMarshalLinks(t *testing.T) {
	// Create test links with various attribute types
	links := []telemetry.LinkData{
		{
			TraceID:    "linked-trace-1",
			SpanID:     "linked-span-1",
			TraceState: "state1",
			Attributes: map[string]any{
				"string1":      "Link1",
				"int1":         int64(123),
				"float1":       float64(9.99),
				"bool1":        true,
				"str_list":     []string{"one", "two", "three"},
				"int_list":     []int64{1, 2, 3},
				"float_list":   []float64{1.1, 2.2, 3.3},
				"bool_list":    []bool{true, false, true},
			},
			DroppedAttributesCount: 0,
		},
		{
			TraceID:    "linked-trace-2",
			SpanID:     "linked-span-2",
			TraceState: "state2",
			Attributes: map[string]any{
				"string2":      "Link2",
				"int2":         int64(456),
				"float2":       float64(12.34),
				"bool2":        false,
				"str_list2":    []string{"a", "b", "c"},
				"int_list2":    []int64{10, 20, 30},
				"float_list2":  []float64{10.1, 20.2, 30.3},
				"bool_list2":   []bool{false, true, false},
			},
			DroppedAttributesCount: 2,
		},
	}

	// Marshal links
	jsonStr := MarshalLinks(links)
	
	// Verify it's valid JSON
	var unmarshaled []map[string]any
	err := json.Unmarshal([]byte(jsonStr), &unmarshaled)
	assert.NoError(t, err, "should be valid JSON")
	assert.Len(t, unmarshaled, 2, "should have 2 links")

	// Check first link
	link1 := unmarshaled[0]
	assert.Equal(t, "linked-trace-1", link1["traceID"])
	assert.Equal(t, "linked-span-1", link1["spanID"])
	assert.Equal(t, "state1", link1["traceState"])
	assert.Equal(t, float64(0), link1["droppedAttributesCount"])

	// Check attributes of first link
	attrs1 := link1["attributes"].(map[string]any)
	assert.Equal(t, "Link1", attrs1["string1"].(map[string]any)["str"])
	assert.Equal(t, float64(123), attrs1["int1"].(map[string]any)["bigint"])
	assert.Equal(t, 9.99, attrs1["float1"].(map[string]any)["double"])
	assert.Equal(t, true, attrs1["bool1"].(map[string]any)["boolean"])
	
	// Check lists
	strList1 := attrs1["str_list"].(map[string]any)["str_list"].([]any)
	assert.Equal(t, "one", strList1[0])
	assert.Equal(t, "two", strList1[1])
	assert.Equal(t, "three", strList1[2])
	
	intList1 := attrs1["int_list"].(map[string]any)["bigint_list"].([]any)
	assert.Equal(t, float64(1), intList1[0])
	assert.Equal(t, float64(2), intList1[1])
	assert.Equal(t, float64(3), intList1[2])
	
	floatList1 := attrs1["float_list"].(map[string]any)["double_list"].([]any)
	assert.Equal(t, 1.1, floatList1[0])
	assert.Equal(t, 2.2, floatList1[1])
	assert.Equal(t, 3.3, floatList1[2])
	
	boolList1 := attrs1["bool_list"].(map[string]any)["boolean_list"].([]any)
	assert.Equal(t, true, boolList1[0])
	assert.Equal(t, false, boolList1[1])
	assert.Equal(t, true, boolList1[2])

	// Check second link
	link2 := unmarshaled[1]
	assert.Equal(t, "linked-trace-2", link2["traceID"])
	assert.Equal(t, "linked-span-2", link2["spanID"])
	assert.Equal(t, "state2", link2["traceState"])
	assert.Equal(t, float64(2), link2["droppedAttributesCount"])
}

func TestFormatAttributes(t *testing.T) {
	// Test with various attribute types
	attributes := map[string]any{
		"string1":      "Hello",
		"int1":         int64(42),
		"float1":       float64(3.14),
		"bool1":        true,
		"str_list":     []string{"one", "two", "three"},
		"int_list":     []int64{1, 2, 3},
		"float_list":   []float64{1.1, 2.2, 3.3},
		"bool_list":    []bool{true, false, true},
		"mystery_meat": struct{}{}, // Should be converted to string
	}

	formatted := formatAttributes(attributes)
	
	// Check string
	assert.Equal(t, "Hello", formatted["string1"]["str"])
	
	// Check int
	assert.Equal(t, int64(42), formatted["int1"]["bigint"])
	
	// Check float
	assert.Equal(t, 3.14, formatted["float1"]["double"])
	
	// Check bool
	assert.Equal(t, true, formatted["bool1"]["boolean"])
	
	// Check string list
	strList := formatted["str_list"]["str_list"].([]string)
	assert.Equal(t, "one", strList[0])
	assert.Equal(t, "two", strList[1])
	assert.Equal(t, "three", strList[2])
	
	// Check int list
	intList := formatted["int_list"]["bigint_list"].([]int64)
	assert.Equal(t, int64(1), intList[0])
	assert.Equal(t, int64(2), intList[1])
	assert.Equal(t, int64(3), intList[2])
	
	// Check float list
	floatList := formatted["float_list"]["double_list"].([]float64)
	assert.Equal(t, 1.1, floatList[0])
	assert.Equal(t, 2.2, floatList[1])
	assert.Equal(t, 3.3, floatList[2])
	
	// Check bool list
	boolList := formatted["bool_list"]["boolean_list"].([]bool)
	assert.Equal(t, true, boolList[0])
	assert.Equal(t, false, boolList[1])
	assert.Equal(t, true, boolList[2])
	
	// Check unknown type (should be converted to string)
	assert.Equal(t, "{}", formatted["mystery_meat"]["str"])
}

func TestMarshalAttributes(t *testing.T) {
	// Test with various attribute types
	attributes := map[string]any{
		"string1":      "Hello",
		"int1":         int64(42),
		"float1":       float64(3.14),
		"bool1":        true,
		"str_list":     []string{"one", "two", "three"},
		"int_list":     []int64{1, 2, 3},
		"float_list":   []float64{1.1, 2.2, 3.3},
		"bool_list":    []bool{true, false, true},
	}

	jsonStr := MarshalAttributes(attributes)
	
	// Verify it's valid JSON
	var unmarshaled map[string]any
	err := json.Unmarshal([]byte(jsonStr), &unmarshaled)
	assert.NoError(t, err, "should be valid JSON")
	
	// Check string
	assert.Equal(t, "Hello", unmarshaled["string1"].(map[string]any)["str"])
	
	// Check int
	assert.Equal(t, float64(42), unmarshaled["int1"].(map[string]any)["bigint"])
	
	// Check float
	assert.Equal(t, 3.14, unmarshaled["float1"].(map[string]any)["double"])
	
	// Check bool
	assert.Equal(t, true, unmarshaled["bool1"].(map[string]any)["boolean"])
	
	// Check string list
	strList := unmarshaled["str_list"].(map[string]any)["str_list"].([]any)
	assert.Equal(t, "one", strList[0])
	assert.Equal(t, "two", strList[1])
	assert.Equal(t, "three", strList[2])
	
	// Check int list - JSON numbers are decoded as float64. This is fine.
	intList := unmarshaled["int_list"].(map[string]any)["bigint_list"].([]any)
	assert.Equal(t, float64(1), intList[0])
	assert.Equal(t, float64(2), intList[1])
	assert.Equal(t, float64(3), intList[2])
	
	// Check float list
	floatList := unmarshaled["float_list"].(map[string]any)["double_list"].([]any)
	assert.Equal(t, 1.1, floatList[0])
	assert.Equal(t, 2.2, floatList[1])
	assert.Equal(t, 3.3, floatList[2])
	
	// Check bool list
	boolList := unmarshaled["bool_list"].(map[string]any)["boolean_list"].([]any)
	assert.Equal(t, true, boolList[0])
	assert.Equal(t, false, boolList[1])
	assert.Equal(t, true, boolList[2])
}

func TestParseRawEvents(t *testing.T) {
	// Create test data that mimics what DuckDB would return
	baseTime := time.Unix(0, 0)
	rawEvents := []any{
		map[string]any{
			"name": "event1",
			"timestamp": baseTime.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			"attributes": map[string]any{
				"string1": map[string]any{"str": "Hello"},
				"int1": map[string]any{"bigint": float64(42)},
				"float1": map[string]any{"double": 3.14},
				"bool1": map[string]any{"boolean": true},
				"str_list": map[string]any{"str_list": []any{"one", "two", "three"}},
				"int_list": map[string]any{"bigint_list": []any{float64(1), float64(2), float64(3)}},
				"float_list": map[string]any{"double_list": []any{1.1, 2.2, 3.3}},
				"bool_list": map[string]any{"boolean_list": []any{true, false, true}},
			},
			"droppedAttributesCount": float64(0),
		},
		map[string]any{
			"name": "event2",
			"timestamp": baseTime.Add(200 * time.Millisecond).Format(time.RFC3339Nano),
			"attributes": map[string]any{
				"string2": map[string]any{"str": "World"},
				"int2": map[string]any{"bigint": float64(100)},
				"float2": map[string]any{"double": 6.28},
				"bool2": map[string]any{"boolean": false},
				"str_list2": map[string]any{"str_list": []any{"a", "b", "c"}},
				"int_list2": map[string]any{"bigint_list": []any{float64(10), float64(20), float64(30)}},
				"float_list2": map[string]any{"double_list": []any{10.1, 20.2, 30.3}},
				"bool_list2": map[string]any{"boolean_list": []any{false, true, false}},
			},
			"droppedAttributesCount": float64(1),
		},
	}

	// Parse events
	events := parseRawEvents(rawEvents)
	
	// Check number of events
	assert.Len(t, events, 2, "should have 2 events")
	
	// Check first event
	event1 := events[0]
	assert.Equal(t, "event1", event1.Name)
	assert.Equal(t, baseTime.Add(100*time.Millisecond).UnixNano(), event1.Timestamp.UnixNano())
	assert.Equal(t, uint32(0), event1.DroppedAttributesCount)
	
	// Check attributes of first event
	assert.Equal(t, "Hello", event1.Attributes["string1"])
	assert.Equal(t, int64(42), event1.Attributes["int1"])
	assert.Equal(t, float64(3.14), event1.Attributes["float1"])
	assert.Equal(t, true, event1.Attributes["bool1"])
	
	// Check lists
	assert.Equal(t, []string{"one", "two", "three"}, event1.Attributes["str_list"])
	assert.Equal(t, []int64{1, 2, 3}, event1.Attributes["int_list"])
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, event1.Attributes["float_list"])
	assert.Equal(t, []bool{true, false, true}, event1.Attributes["bool_list"])
	
	// Check second event
	event2 := events[1]
	assert.Equal(t, "event2", event2.Name)
	assert.Equal(t, baseTime.Add(200*time.Millisecond).UnixNano(), event2.Timestamp.UnixNano())
	assert.Equal(t, uint32(1), event2.DroppedAttributesCount)
}

func TestParseRawLinks(t *testing.T) {
	// Create test data that mimics what DuckDB would return
	rawLinks := []any{
		map[string]any{
			"traceID": "linked-trace-1",
			"spanID": "linked-span-1",
			"traceState": "state1",
			"attributes": map[string]any{
				"string1": map[string]any{"str": "Link1"},
				"int1": map[string]any{"bigint": float64(123)},
				"float1": map[string]any{"double": 9.99},
				"bool1": map[string]any{"boolean": true},
				"str_list": map[string]any{"str_list": []any{"one", "two", "three"}},
				"int_list": map[string]any{"bigint_list": []any{float64(1), float64(2), float64(3)}},
				"float_list": map[string]any{"double_list": []any{1.1, 2.2, 3.3}},
				"bool_list": map[string]any{"boolean_list": []any{true, false, true}},
			},
			"droppedAttributesCount": float64(0),
		},
		map[string]any{
			"traceID": "linked-trace-2",
			"spanID": "linked-span-2",
			"traceState": "state2",
			"attributes": map[string]any{
				"string2": map[string]any{"str": "Link2"},
				"int2": map[string]any{"bigint": float64(456)},
				"float2": map[string]any{"double": 12.34},
				"bool2": map[string]any{"boolean": false},
				"str_list2": map[string]any{"str_list": []any{"a", "b", "c"}},
				"int_list2": map[string]any{"bigint_list": []any{float64(10), float64(20), float64(30)}},
				"float_list2": map[string]any{"double_list": []any{10.1, 20.2, 30.3}},
				"bool_list2": map[string]any{"boolean_list": []any{false, true, false}},
			},
			"droppedAttributesCount": float64(2),
		},
	}

	// Parse links
	links := parseRawLinks(rawLinks)
	
	// Check number of links
	assert.Len(t, links, 2, "should have 2 links")
	
	// Check first link
	link1 := links[0]
	assert.Equal(t, "linked-trace-1", link1.TraceID)
	assert.Equal(t, "linked-span-1", link1.SpanID)
	assert.Equal(t, "state1", link1.TraceState)
	assert.Equal(t, uint32(0), link1.DroppedAttributesCount)
	
	// Check attributes of first link
	assert.Equal(t, "Link1", link1.Attributes["string1"])
	assert.Equal(t, int64(123), link1.Attributes["int1"])
	assert.Equal(t, float64(9.99), link1.Attributes["float1"])
	assert.Equal(t, true, link1.Attributes["bool1"])
	
	// Check lists
	assert.Equal(t, []string{"one", "two", "three"}, link1.Attributes["str_list"])
	assert.Equal(t, []int64{1, 2, 3}, link1.Attributes["int_list"])
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, link1.Attributes["float_list"])
	assert.Equal(t, []bool{true, false, true}, link1.Attributes["bool_list"])
	
	// Check second link
	link2 := links[1]
	assert.Equal(t, "linked-trace-2", link2.TraceID)
	assert.Equal(t, "linked-span-2", link2.SpanID)
	assert.Equal(t, "state2", link2.TraceState)
	assert.Equal(t, uint32(2), link2.DroppedAttributesCount)
}

func TestParseRawAttributes(t *testing.T) {
	// Create test data that mimics what DuckDB would return
	rawAttributes := map[string]any{
		"string1": map[string]any{"str": "Hello"},
		"int1": map[string]any{"bigint": float64(42)},
		"float1": map[string]any{"double": 3.14},
		"bool1": map[string]any{"boolean": true},
		"str_list": map[string]any{"str_list": []any{"one", "two", "three"}},
		"int_list": map[string]any{"bigint_list": []any{float64(1), float64(2), float64(3)}},
		"float_list": map[string]any{"double_list": []any{1.1, 2.2, 3.3}},
		"bool_list": map[string]any{"boolean_list": []any{true, false, true}},
	}

	// Parse attributes
	attributes := parseRawAttributes(rawAttributes)
	
	// Check string
	assert.Equal(t, "Hello", attributes["string1"])
	
	// Check int
	assert.Equal(t, int64(42), attributes["int1"])
	
	// Check float
	assert.Equal(t, float64(3.14), attributes["float1"])
	
	// Check bool
	assert.Equal(t, true, attributes["bool1"])
	
	// Check lists
	assert.Equal(t, []string{"one", "two", "three"}, attributes["str_list"])
	assert.Equal(t, []int64{1, 2, 3}, attributes["int_list"])
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, attributes["float_list"])
	assert.Equal(t, []bool{true, false, true}, attributes["bool_list"])
}
