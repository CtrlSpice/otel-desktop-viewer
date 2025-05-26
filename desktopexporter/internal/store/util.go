package store

import (
	"fmt"
	"log"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/marcboeker/go-duckdb/v2"
)

// dbEvent represents an event in DuckDB format.
// NOTE: The `db` struct tags are absolutely required! They map Go struct fields
// to the correct DuckDB column names. Without them, the database operations will fail.
type dbEvent struct {
	Name                   string `db:"name"`
	Timestamp              time.Time `db:"timestamp"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32 `db:"droppedAttributesCount"`
}

// duckDBLink represents a link in DuckDB format.
// NOTE: The `db` struct tags are absolutely required! They map Go struct fields
// to the correct DuckDB column names. Without them, the database operations will fail.
type duckDBLink struct {
	TraceID                string `db:"traceID"`
	SpanID				   string `db:"spanID"`
	TraceState             string `db:"traceState"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32 `db:"droppedAttributesCount"`
}

// Helper function to convert map[string]any to duckdb.Map for attributes
// This function creates proper DuckDB union types with correct tags to preserve type information
func toDuckDBMap(attributes map[string]any) duckdb.Map {
	duckDBMap := duckdb.Map{}
	
	for attributeName, attributeValue := range attributes {
		tag := ""
		value := attributeValue

		switch t := attributeValue.(type) {
		case string:
			tag = "str"
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			tag = "bigint"
		case float32, float64:
			tag = "double"
		case bool:
			tag = "boolean"
		case []string:
			tag = "str_list"
		case []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64:
			tag = "bigint_list"
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
				log.Printf("unsupported list attribute %s was unceremoniously cast to []string: %v", attributeName, err)	
			} else {
				tag = derivedTag
			}
		default:
			tag = "str"
			value = fmt.Sprintf("%v", attributeValue)
			log.Printf("unsupported attribute type was unceremoniously cast to string. name: %s type: %T value: %v", attributeName, t, attributeValue)
		}

		duckDBMap[attributeName] = duckdb.Union{Tag: tag, Value: value}
	}
	
	return duckDBMap
}

// Helper function to convert []telemetry.EventData to []duckDBEvent
func toDuckDBEvents(events []telemetry.EventData) []dbEvent {
	dbEvents := []dbEvent{}
	
	for _, event := range events {
		dbe := dbEvent{
			Name:                   event.Name,
			Timestamp:              event.Timestamp,
			Attributes:             toDuckDBMap(event.Attributes),
			DroppedAttributesCount: event.DroppedAttributesCount,
		}

		dbEvents = append(dbEvents, dbe)
	}
	return dbEvents
}

// Helper function to convert []telemetry.LinkData to []duckDBLink
func toDuckDBLinks(links []telemetry.LinkData) []duckDBLink {
	if len(links) == 0 {
		return []duckDBLink{}
	}
	duckDBLinks := []duckDBLink{}
	for _, link := range links {
		duckDBLink := duckDBLink{
			TraceID:                link.TraceID,
			SpanID:                 link.SpanID,
			TraceState:             link.TraceState,
			Attributes:             toDuckDBMap(link.Attributes),
			DroppedAttributesCount: link.DroppedAttributesCount,
		}
		duckDBLinks = append(duckDBLinks, duckDBLink)
	}
	return duckDBLinks
}

// Helper function to parse raw attributes from DuckDB format
// This function properly extracts values from DuckDB unions while preserving type information
func fromDuckDBMap(rawAttributes map[string]duckdb.Union) map[string]any {
	attributes := map[string]any{}

	for attrName, union := range rawAttributes {
		// The union.Value already contains the properly typed value
		// DuckDB's union handling preserves the original types
		attributes[attrName] = union.Value
	}
	
	return attributes
}

// getListTypeTag examines the elements of a []any slice and returns the appropriate type tag
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
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			if checkListHomogeneity[int64](list) {
				typeTag = "bigint_list"
			} else {
				return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
			}
		case float32, float64:
			if checkListHomogeneity[float64](list) {
				typeTag = "double_list"
			} else {
				return typeTag, fmt.Errorf("list attribute contains mixed types: %v", list)
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