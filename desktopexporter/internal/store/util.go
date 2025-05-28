package store

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/marcboeker/go-duckdb/v2"
)

// Error message constants for attribute type validation
const (
	errMixedTypesPrefix      = "list attribute contains mixed types: "
	errNilValue              = errMixedTypesPrefix + "list contains nil value"
	errIncompatibleType      = errMixedTypesPrefix + "incompatible type %T"
	errIncompatibleIntType   = errMixedTypesPrefix + "incompatible type %T in integer list"
	errIncompatibleFloatType = errMixedTypesPrefix + "incompatible type %T in float list"
	errUint64Overflow        = "uint64 value %d exceeds int64 range"
	errNilFirstElement       = "nil value in list attribute"
	errUnsupportedListType   = "unsupported list attribute type: %T"
)

type dbEvent struct {
	Name                   string     `db:"name"`
	Timestamp              time.Time  `db:"timestamp"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32     `db:"droppedAttributesCount"`
}


type dbLink struct {
	TraceID                string     `db:"traceID"`
	SpanID                 string     `db:"spanID"`
	TraceState             string     `db:"traceState"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32     `db:"droppedAttributesCount"`
}

func toDbMap(attributes map[string]any) duckdb.Map {
	dbMap := duckdb.Map{}

	for attributeName, attributeValue := range attributes {
		tag := ""
		value := attributeValue

		switch t := attributeValue.(type) {
		case string:
			tag = "str"
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
			tag = "bigint"
		case uint64:
			if convertedValue, hasOverflow := stringifyOnOverflow(attributeName, t); hasOverflow {
				tag = "str"
				value = convertedValue
			} else {
				tag = "bigint"
			}
		case float32, float64:
			tag = "double"
		case bool:
			tag = "boolean"
		case []string:
			tag = "str_list"
		case []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32:
			tag = "bigint_list"
		case []uint64:
			if strList, hasOverflow := stringifyOnOverflow(attributeName, t...); hasOverflow {
				tag = "str_list"
				value = strList
			} else {
				tag = "bigint_list"
			}
		case []float32, []float64:
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
				log.Printf("unsupported list attribute %s was converted to []string: %v", attributeName, err)
			} else {
				tag = derivedTag
			}
		default:
			tag = "str"
			value = fmt.Sprintf("%v", attributeValue)
			log.Printf("unsupported attribute type was unceremoniously cast to string. name: %s type: %T value: %v", attributeName, t, attributeValue)
		}

		dbMap[attributeName] = duckdb.Union{Tag: tag, Value: value}
	}

	return dbMap
}

func toDbEvents(events []telemetry.EventData) []dbEvent {
	dbEvents := []dbEvent{}

	for _, event := range events {
		dbe := dbEvent{
			Name:                   event.Name,
			Timestamp:              event.Timestamp,
			Attributes:             toDbMap(event.Attributes),
			DroppedAttributesCount: event.DroppedAttributesCount,
		}

		dbEvents = append(dbEvents, dbe)
	}
	return dbEvents
}

func toDbLinks(links []telemetry.LinkData) []dbLink {
	if len(links) == 0 {
		return []dbLink{}
	}
	dbLinks := []dbLink{}
	for _, link := range links {
		dbLink := dbLink{
			TraceID:                link.TraceID,
			SpanID:                 link.SpanID,
			TraceState:             link.TraceState,
			Attributes:             toDbMap(link.Attributes),
			DroppedAttributesCount: link.DroppedAttributesCount,
		}
		dbLinks = append(dbLinks, dbLink)
	}
	return dbLinks
}

func fromDbMap(rawAttributes map[string]duckdb.Union) map[string]any {
	attributes := map[string]any{}

	for attrName, union := range rawAttributes {
		attributes[attrName] = union.Value
	}

	return attributes
}

func fromDbEvents(dbEvents []dbEvent) []telemetry.EventData {
	events := []telemetry.EventData{}

	for _, dbEvent := range dbEvents {
		attributes := map[string]any{}
		for k, v := range dbEvent.Attributes {
			if name, ok := k.(string); ok {
				if union, ok := v.(duckdb.Union); ok {
					attributes[name] = union.Value
				}
			}
		}

		event := telemetry.EventData{
			Name:                   dbEvent.Name,
			Timestamp:              dbEvent.Timestamp,
			Attributes:             attributes,
			DroppedAttributesCount: dbEvent.DroppedAttributesCount,
		}
		events = append(events, event)
	}
	return events
}

func fromDbLinks(dbLinks []dbLink) []telemetry.LinkData {
	links := []telemetry.LinkData{}

	for _, dbLink := range dbLinks {
		attributes := map[string]any{}
		for k, v := range dbLink.Attributes {
			if name, ok := k.(string); ok {
				if union, ok := v.(duckdb.Union); ok {
					attributes[name] = union.Value
				}
			}
		}

		link := telemetry.LinkData{
			TraceID:                dbLink.TraceID,
			SpanID:                 dbLink.SpanID,
			TraceState:             dbLink.TraceState,
			Attributes:             attributes,
			DroppedAttributesCount: dbLink.DroppedAttributesCount,
		}
		links = append(links, link)
	}

	return links
}

// getListTypeTag examines the elements of a []any slice and returns the appropriate type tag
// in order to store our list as a type supported by our attribute UNION in DuckDB.
func getListTypeTag(list []any) (string, error) {
	tag := "str_list" // Default fallback for mixed types
	if len(list) == 0 {
		// Empty arrays are valid per OpenTelemetry spec - default to string list for storage.
		return "str_list", nil
	}

	if list[0] == nil {
		return tag, fmt.Errorf(errNilFirstElement)
	}

	switch list[0].(type) {
	case string:
		if err := validateUniformList[string](list); err != nil {
			return tag, err
		}
		return "str_list", nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if err := validateUniformList[int64](list); err != nil {
			return tag, err
		}
		return "bigint_list", nil
	case float32, float64:
		if err := validateUniformList[float64](list); err != nil {
			return tag, err
		}
		return "double_list", nil
	case bool:
		if err := validateUniformList[bool](list); err != nil {
			return tag, err
		}
		return "boolean_list", nil
	default:
		return tag, fmt.Errorf(errUnsupportedListType, list[0])
	}
}

// validateUniformList validates the homogeneity of a list attribute to conform with OTel spec.
func validateUniformList[T any](list []any) error {
	var zero T

	for _, item := range list {
		if item == nil {
			return fmt.Errorf(errNilValue)
		}

		// Handle integer types specially to check for uint64 overflow
		switch any(zero).(type) {
		case int64:
			switch val := item.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
				// These all fit safely in int64
			case uint64:
				// Check for overflow: uint64 values > math.MaxInt64 can't fit in int64
				if val > math.MaxInt64 {
					return fmt.Errorf(errUint64Overflow, val)
				}
			default:
				return fmt.Errorf(errIncompatibleIntType, item)
			}
		case float64:
			switch item.(type) {
			case float32, float64:
				// All float types can be converted to float64
			default:
				return fmt.Errorf(errIncompatibleFloatType, item)
			}
		default:
			// For all other types, use standard type assertion
			if _, ok := item.(T); !ok {
				return fmt.Errorf(errIncompatibleType, item)
			}
		}
	}
	return nil
}

// Note: This is very unlikely to happen in practice, but it's a good idea to have a fallback for when it does.
// stringifyOnOverflow checks uint64 values for overflow beyond math.MaxInt64.
// For individual values: returns (stringValue, hasOverflow) - if no overflow, returns ("", false)
// For slices: returns ([]string, hasOverflow) if any value overflows, otherwise (nil, false)
func stringifyOnOverflow(attributeName string, values ...uint64) (any, bool) {
	// Check if any uint64 values exceed int64 range
	hasOverflow := false
	for _, val := range values {
		if val > math.MaxInt64 {
			hasOverflow = true
			break
		}
	}

	// No overflow - return original value for single, nil for slice
	if !hasOverflow {
		if len(values) == 1 {
			return values[0], false
		}
		return nil, false
	}

	// Has overflow - convert to string
	if len(values) == 1 {
		strVal := fmt.Sprintf("%v", values[0])
		log.Printf("uint64 attribute %s with value %d exceeds int64 range and was converted to string", attributeName, values[0])
		return strVal, true
	}

	// Multiple values - convert entire slice to strings
	strList := make([]string, len(values))
	for i, val := range values {
		strList[i] = fmt.Sprintf("%v", val)
	}
	log.Printf("[]uint64 attribute %s contains values exceeding int64 range and was converted to []string", attributeName)
	return strList, true
}
