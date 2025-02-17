package store

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// Helper function to convert Events to DuckDB list of STRUCT string format
func eventToString(events []telemetry.EventData) string {
	eventStrings := []string{}

	for _, event := range events {
		attributes := mapToString(event.Attributes)
		eventStrings = append(eventStrings, fmt.Sprintf("{name: '%s', timestamp: '%v', attributes: %s, droppedAttributesCount: %d}",
			escapeString(event.Name),
			event.Timestamp.Format(time.RFC3339Nano),
			attributes,
			event.DroppedAttributesCount))
	}
	return fmt.Sprintf("[%s]", strings.Join(eventStrings, ", "))
}

// Helper function to convert Links to DuckDB list of STRUCT string format
func linkToString(links []telemetry.LinkData) string {
	linkStrings := []string{}

	for _, link := range links {
		attributes := mapToString(link.Attributes)
		linkStrings = append(linkStrings, fmt.Sprintf(
			"{traceID: '%s', spanID: '%s', traceState: '%s', attributes: %s, droppedAttributesCount: %d}",
			escapeString(link.TraceID),
			escapeString(link.SpanID),
			escapeString(link.TraceState),
			attributes,
			link.DroppedAttributesCount))
	}
	return fmt.Sprintf("[%s]", strings.Join(linkStrings, ", "))
}

// Helper function to convert map to DuckDB MAP string format
func mapToString(m map[string]interface{}) string {
	var pairs []string
	for k, v := range m {
		var valStr string
		switch v := v.(type) {
		case string:
			valStr = fmt.Sprintf("'%s'::attribute", escapeString(v))
		case int, int32, int64:
			valStr = fmt.Sprintf("%d::attribute", v)
		case float32, float64:
			valStr = fmt.Sprintf("%f::attribute", v)
		case bool:
			if v {
				valStr = "true::attribute"
			} else {
				valStr = "false::attribute"
			}
		case []string:
			elements := make([]string, len(v))
			for i, s := range v {
				elements[i] = fmt.Sprintf("'%s'::attribute", escapeString(s))
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []int64:
			elements := make([]string, len(v))
			for i, n := range v {
				elements[i] = fmt.Sprintf("%d::attribute", n)
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []float64:
			elements := make([]string, len(v))
			for i, f := range v {
				elements[i] = fmt.Sprintf("%f::attribute", f)
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []bool:
			elements := make([]string, len(v))
			for i, b := range v {
				if b {
					elements[i] = "true::attribute"
				} else {
					elements[i] = "false::attribute"
				}
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		default:
			valStr = fmt.Sprintf("union_value(str := '%v')", v)
		}
		pairs = append(pairs, fmt.Sprintf("'%s': %v", escapeString(k), valStr))
	}
	return fmt.Sprintf("MAP{%s}", strings.Join(pairs, ", "))
}

// Helper function to parse events from raw DuckDB [STRUCT(...)] string format
func parseRawEvents(rawEvents string) []telemetry.EventData {
	if rawEvents == "" || rawEvents == "[]" {
		return []telemetry.EventData{}
	}

	// Remove outer brackets
	rawEvents = strings.Trim(rawEvents, "[]")
	if rawEvents == "" {
		return []telemetry.EventData{}
	}

	var events []telemetry.EventData
	// Split on "}, {" to separate individual events
	rawEventsList := strings.Split(rawEvents, "}, {")

	for _, rawEvent := range rawEventsList {
		// Clean up the event string
		rawEvent = strings.Trim(rawEvent, "{}")

		// Split into fields
		fields := strings.Split(rawEvent, ", ")

		event := telemetry.EventData{
			Attributes: make(map[string]interface{}),
		}

		for _, field := range fields {
			key, value, found := strings.Cut(field, ": ")
			if !found {
				continue
			}

			key = strings.Trim(key, "'")
			value = strings.Trim(value, "'")

			switch key {
			case "name":
				event.Name = value
			case "timestamp":
				// Parse timestamp
				if t, err := time.Parse("2006-01-02 15:04:05.999999999", value); err == nil {
					event.Timestamp = t
				}
			case "attributes":
				event.Attributes = parseRawAttributes(value)
			case "droppedAttributesCount":
				if count, err := strconv.ParseUint(value, 10, 32); err == nil {
					event.DroppedAttributesCount = uint32(count)
				}
			default:
				log.Printf("unsupported event field was ignored. key: %s value: %s", key, value)
			}
		}

		events = append(events, event)
	}

	return events
}

// Helper function to parse links from raw DuckDB [STRUCT(...)] string format
func parseRawLinks(rawLinks string) []telemetry.LinkData {
	if rawLinks == "" || rawLinks == "[]" {
		return []telemetry.LinkData{}
	}

	// Remove outer brackets
	rawLinks = strings.Trim(rawLinks, "[]")
	if rawLinks == "" {
		return []telemetry.LinkData{}
	}

	var links []telemetry.LinkData
	// Split on "}, {" to separate individual links
	rawLinksList := strings.Split(rawLinks, "}, {")

	for _, rawLink := range rawLinksList {
		// Clean up the link string
		rawLink = strings.Trim(rawLink, "{}")

		// Split into fields
		fields := strings.Split(rawLink, ", ")

		link := telemetry.LinkData{
			Attributes: make(map[string]interface{}),
		}

		for _, field := range fields {
			key, value, found := strings.Cut(field, ": ")
			if !found {
				continue
			}

			key = strings.Trim(key, "'")
			value = strings.Trim(value, "'")

			switch key {
			case "traceID":
				link.TraceID = value
			case "spanID":
				link.SpanID = value
			case "traceState":
				link.TraceState = value
			case "attributes":
				link.Attributes = parseRawAttributes(value)
			case "droppedAttributesCount":
				if count, err := strconv.ParseUint(value, 10, 32); err == nil {
					link.DroppedAttributesCount = uint32(count)
				}
			default:
				log.Printf("unsupported link field was ignored. key: %s value: %s", key, value)
			}
		}

		links = append(links, link)
	}

	return links
}

// Helper function to parse raw attributes from DuckDB MAP string format, preserving types
func parseRawAttributes(rawAttributes string) map[string]interface{} {
	attributes := make(map[string]interface{})

	// Trim the outer braces first
	rawAttributes = strings.Trim(rawAttributes, "{}")
	if rawAttributes == "" {
		return attributes
	}

	pairs := strings.Split(rawAttributes, ", ")

	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		// Handle array values
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			arrayStr := strings.Trim(value, "[]")
			if arrayStr == "" {
				attributes[key] = []interface{}{}
				continue
			}

			var elements []string
			var current string
			inQuotes := false

			// Split preserving quoted strings
			for _, r := range arrayStr {
				switch r {
				case '\'':
					inQuotes = !inQuotes
				case ' ':
					if !inQuotes {
						if current != "" {
							elements = append(elements, current)
							current = ""
						}
					} else {
						current += string(r)
					}
				default:
					current += string(r)
				}
			}
			if current != "" {
				elements = append(elements, current)
			}

			if len(elements) > 0 {
				// Try as integers first
				ints := make([]int64, len(elements))
				allInts := true
				for i, e := range elements {
					if n, err := strconv.ParseInt(e, 10, 64); err == nil {
						ints[i] = n
					} else {
						allInts = false
						break
					}
				}
				if allInts {
					attributes[key] = ints
					continue
				}

				// Then try to parse as float array
				floats := make([]float64, len(elements))
				allFloats := true
				for i, e := range elements {
					if f, err := strconv.ParseFloat(e, 64); err == nil {
						floats[i] = f
					} else {
						allFloats = false
						break
					}
				}
				if allFloats {
					attributes[key] = floats
					continue
				}

				// Try to parse as boolean array first
				if elements[0] == "true" || elements[0] == "false" {
					bools := make([]bool, len(elements))
					allBools := true
					for i, e := range elements {
						if b, err := strconv.ParseBool(e); err == nil {
							bools[i] = b
						} else {
							allBools = false
							break
						}
					}
					if allBools {
						attributes[key] = bools
						continue
					}
				}

				// If none of the above, it's a string array
				strArray := make([]string, len(elements))
				for i := range elements {
					strArray[i] = strings.Trim(elements[i], "'")
				}
				attributes[key] = strArray
			}
			continue
		}

		// Handle scalar values
		value = strings.Trim(value, "'")
		if value == "true" || value == "false" {
			b, _ := strconv.ParseBool(value)
			attributes[key] = b
			continue
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			attributes[key] = n
			continue
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			attributes[key] = f
			continue
		}

		// Default to string
		attributes[key] = value
	}
	return attributes
}

// Helper function to escape single quotes in strings for SQL
func escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
