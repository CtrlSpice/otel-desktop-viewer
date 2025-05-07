package store

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// Helper function to convert Events to JSON format
func MarshalEvents(events []telemetry.EventData) string {
	type formattedEvent struct {
		Name string `json:"name"`
		Timestamp string `json:"timestamp"`
		Attributes map[string]map[string]any `json:"attributes"`
		DroppedAttributesCount uint32 `json:"droppedAttributesCount"`
	}

	formattedEvents := []formattedEvent{}

	for _, event := range events {
		fe := formattedEvent{
			Name: event.Name,
			Timestamp: event.Timestamp.Format(time.RFC3339Nano),
			Attributes: formatAttributes(event.Attributes),
			DroppedAttributesCount: event.DroppedAttributesCount,
		}
		formattedEvents = append(formattedEvents, fe)
	}

	jsonEvents, err := json.Marshal(formattedEvents)
	if err != nil {
		log.Printf("error marshalling events %v: %v", events, err)
		return "[]"
	}
	return string(jsonEvents)
}

// Helper function to convert Links to JSON format
func MarshalLinks(links []telemetry.LinkData) string {
	type formattedLink struct {
		TraceID string `json:"traceID"`
		SpanID string `json:"spanID"`
		TraceState string `json:"traceState"`
		Attributes map[string]map[string]any `json:"attributes"`
		DroppedAttributesCount uint32 `json:"droppedAttributesCount"`
	}

	formattedLinks := []formattedLink{}

	for _, link := range links {
		fl := formattedLink{
			TraceID: link.TraceID,
			SpanID: link.SpanID,
			TraceState: link.TraceState,
			Attributes: formatAttributes(link.Attributes),
			DroppedAttributesCount: link.DroppedAttributesCount,
		}
		formattedLinks = append(formattedLinks, fl)
	}

	jsonLinks, err := json.Marshal(formattedLinks)
	if err != nil {
		log.Printf("error marshalling links %v: %v", links, err)
		return "[]"
	}

	return string(jsonLinks)
}

// Helper function to convert our attributes to a map of types compatible with DuckDB's UNION type
// What go-duckdb doesn't know can't hurt it
func formatAttributes(attributes map[string]any) map[string]map[string]any {
	// This is how DuckDB outputs a map of UNION types converted to JSON. I'd have done it differently, but it's what it is.
	rawAttributes := map[string]map[string]any{}
	
	for attributeName, attributeValue := range attributes {
		tag := ""
		value := attributeValue

		switch t := attributeValue.(type) {
		case string:
			tag = "str"
		case int, int32, int64:
			tag = "bigint"
		case float32, float64:
			tag = "double"
		case bool:
			tag = "boolean"
		case []string:
			tag = "str_list"
		case []int64:
			tag = "bigint_list"
		case []float64:
			tag = "double_list"
		case []bool:
			tag = "boolean_list"
		case []any:
			derivedTag, err := getListTypeTag(t)
			if err != nil {
				tag = "str_list"
				strList := []string{}
				for _, v := range t {
					strList = append(strList, fmt.Sprintf("%v", v))
				}
				value = strList
				log.Printf("unsupported list attribute %s was unceremoniously cast to []string: %v", attributeName, err)	
			} else {
				tag = derivedTag
			}
		default:
			tag = "str"
			value = fmt.Sprintf("%v", attributeValue)
			log.Printf("unsupported attribute type was unceremoniously cast to string. name: %s type: %T value: %v", attributeName, t, attributeValue)
		}

		rawAttributes[attributeName] = map[string]any{
			tag: value,
		}
	}
	return rawAttributes
}

// DetermineListType examines the elements of a []any slice and returns the appropriate type tag
func getListTypeTag(list []any) (string, error) {
	typeTag := ""
    if len(list) == 0 {	
        return typeTag, fmt.Errorf("empty list attribute: %v", list)
    }

    if list[0] == nil {
        return typeTag, fmt.Errorf("nil value in list attribute: %v", list)
    }

    switch list[0].(type) {
		case string:
			if checkListHomogeneity[string](list) {
				typeTag = "str_list"
			} else {
				return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
			}
		case float64:
			if f := list[0].(float64); float64(int64(f)) == f {
				if checkListHomogeneity[int64](list) {	
					typeTag = "bigint_list"
				} else {
					return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
				}
			} else {
				if checkListHomogeneity[float64](list) {
					typeTag = "double_list"
				} else {
					return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
				}
			}
		case bool:
			if checkListHomogeneity[bool](list) {
				typeTag = "boolean_list"
			} else {
				return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
			}
		default:
			return typeTag, fmt.Errorf("unsupported list attribute type: %T", list[0])
	}
    
    return typeTag, nil
}

// checkListHomogeneity verifies that all elements in the list can be type asserted to T
func checkListHomogeneity[T any](list []any) bool {
    for _, item := range list {
        if item == nil {
            return false
        }
        if _, ok := item.(T); !ok {
            return false
        }
    }
    return true
}

// Helper function to convert our attributes to JSON format - this is done separately 
// from formatAttributes so we can use the same function when marshalling both events and links
func MarshalAttributes(attributes map[string]any) string {
	formattedAttributes := formatAttributes(attributes)
	jsonAttributes, err := json.Marshal(formattedAttributes)
	if err != nil {
		log.Printf("error marshalling attributes %v: %v", attributes, err)
		return "{}"
	}
	return string(jsonAttributes)
}

// Helper function to parse events from raw DuckDB JSON format
func parseRawEvents(rawEvents []any) []telemetry.EventData {
	if rawEvents == nil {
		return []telemetry.EventData{}
	}

	events := make([]telemetry.EventData, 0, len(rawEvents))
	
	for _, rawEvent := range rawEvents {
		eventMap, ok := rawEvent.(map[string]any)
		if !ok {
			log.Printf("parseRawEvents - Failed to convert raw event to map: %v", rawEvent)
			continue
		}
		
		event := telemetry.EventData{
			Attributes: map[string]any{},
		}
		
		// Extract name
		if name, ok := eventMap["name"].(string); ok {
			event.Name = name
		}
		
		// Extract timestamp
		if timestampStr, ok := eventMap["timestamp"].(string); ok {
			// DuckDB returns timestamps in format "2025-04-24 19:35:32.718466"
			if t, err := time.Parse("2006-01-02 15:04:05.999999", timestampStr); err == nil {
				// Convert UTC time back to local time
				event.Timestamp = t.In(time.Local)
			} else {
				// Fallback to RFC3339Nano if the format doesn't match
				if t, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
					event.Timestamp = t
				} else {
					log.Printf("Failed to parse timestamp: %v", err)
				}
			}
		} else {
			log.Printf("Timestamp not found or not a string: %v", eventMap["timestamp"])
		}
		
		// Extract attributes
		if attrs, ok := eventMap["attributes"].(map[string]any); ok {
			event.Attributes = parseRawAttributes(attrs)
		}
		
		// Extract droppedAttributesCount
		if count, ok := eventMap["droppedAttributesCount"].(float64); ok {
			event.DroppedAttributesCount = uint32(count)
		}
		
		events = append(events, event)
	}
	
	return events
}

// Helper function to parse links from raw DuckDB JSON format
func parseRawLinks(rawLinks []any) []telemetry.LinkData {
	if rawLinks == nil {
		return []telemetry.LinkData{}
	}

	links := make([]telemetry.LinkData, 0, len(rawLinks))
	
	for _, rawLink := range rawLinks {
		linkMap, ok := rawLink.(map[string]any)
		if !ok {
			continue
		}
		
		link := telemetry.LinkData{
			Attributes: map[string]any{},
		}
		
		// Extract traceID
		if traceID, ok := linkMap["traceID"].(string); ok {
			link.TraceID = traceID
		}
		
		// Extract spanID
		if spanID, ok := linkMap["spanID"].(string); ok {
			link.SpanID = spanID
		}
		
		// Extract traceState
		if traceState, ok := linkMap["traceState"].(string); ok {
			link.TraceState = traceState
		}
		
		// Extract attributes
		if attrs, ok := linkMap["attributes"].(map[string]any); ok {
			link.Attributes = parseRawAttributes(attrs)
		}
		
		// Extract droppedAttributesCount
		if count, ok := linkMap["droppedAttributesCount"].(float64); ok {
			link.DroppedAttributesCount = uint32(count)
		}
		
		links = append(links, link)
	}
	
	return links
}

// Helper function to parse raw attributes from DuckDB JSON format
func parseRawAttributes(rawAttributes map[string]any) map[string]any {
	attributes := map[string]any{}
	
	if rawAttributes == nil {
		return attributes
	}

	// Process each attribute
	for attrName, typeValuePair := range rawAttributes {
		// KeyValuePair is the JSON mapping of our UNION type:
		// the key is the type tag, the value is the actual attribute value
		typeValueMap, ok := typeValuePair.(map[string]any)
		if !ok {
			continue
		}
		
		for typeTag, value := range typeValueMap {
			switch typeTag {
			case "str":
				attributes[attrName] = value
			case "bigint":
				// JSON numbers are decoded as float64
				attributes[attrName] = int64(value.(float64))
			case "double":
				attributes[attrName] = value
			case "boolean":
				attributes[attrName] = value
			case "str_list":
				strList := []string{}
				for _, v := range value.([]any){
					strList = append(strList, v.(string))
				}
				attributes[attrName] = strList
			case "bigint_list":
				intList := []int64{}
				for _, v := range value.([]any){
					// JSON numbers are decoded as float64
					intList = append(intList, int64(v.(float64)))
				}
				attributes[attrName] = intList
			case "double_list":
				floatList := []float64{}
				for _, v := range value.([]any){
					floatList = append(floatList, v.(float64))
				}
				attributes[attrName] = floatList
			case "boolean_list":
				boolList := []bool{}
				for _, v := range value.([]any){
					boolList = append(boolList, v.(bool))
				}
				attributes[attrName] = boolList
			default:
				// If we don't recognize the type, cast to string
				log.Printf("unsupported attribute type was unceremoniously cast to string. type: %s value: %v", typeTag, value)
				attributes[attrName] = fmt.Sprintf("%v", value)
			}
		}
	}
	
	return attributes
}
