package attributes

import (
	"database/sql/driver"
	"fmt"
	"log"
	"math"
	"reflect"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/errors"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/util"
	"github.com/marcboeker/go-duckdb/v2"
)

// An Attribute is a key-value pair, which MUST have the following properties:
// - The attribute key MUST be a non-null and non-empty string.
// - Case sensitivity of keys is preserved. Keys that differ in casing are treated as distinct keys.
// - The attribute value is either:
//   - A primitive type: string, boolean, double precision floating point (IEEE 754-1985) or signed 64 bit integer.
//   - An array of primitive type values. The array MUST be homogeneous, i.e., it MUST NOT contain values of different types.
//
// Attributes implements the sql.Scanner and driver.Valuer interfaces to allow easy conversion to and from DuckDB Map types.
type Attributes map[string]any

// Value converts a map of attributes to a DuckDB Map type.
// For uint64 values, if they exceed math.MaxInt64, they are converted to strings.
// For []uint64 values, if any value exceeds math.MaxInt64, the entire slice is converted to []string.
func (attrs Attributes) Value() (driver.Value, error) {
	dbMap := duckdb.Map{}

	for attributeName, attributeValue := range attrs {
		switch t := attributeValue.(type) {
		case string:
			dbMap[attributeName] = duckdb.Union{Tag: "str", Value: t}
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
			dbMap[attributeName] = duckdb.Union{Tag: "bigint", Value: t}
		case uint64:
			value, hasOverflow := util.StringifyOnOverflow(attributeName, t)
			if hasOverflow {
				dbMap[attributeName] = duckdb.Union{Tag: "str", Value: value}
			} else {
				dbMap[attributeName] = duckdb.Union{Tag: "bigint", Value: value}
			}
		case float32, float64:
			dbMap[attributeName] = duckdb.Union{Tag: "double", Value: t}
		case bool:
			dbMap[attributeName] = duckdb.Union{Tag: "boolean", Value: t}
		case []string:
			dbMap[attributeName] = duckdb.Union{Tag: "str_list", Value: t}
		case []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32:
			dbMap[attributeName] = duckdb.Union{Tag: "bigint_list", Value: t}
		case []uint64:
			value, hasOverflow := util.StringifySliceOnOverflow(attributeName, t)
			if hasOverflow {
				dbMap[attributeName] = duckdb.Union{Tag: "str_list", Value: value}
			} else {
				dbMap[attributeName] = duckdb.Union{Tag: "bigint_list", Value: value}
			}
		case []float32, []float64:
			dbMap[attributeName] = duckdb.Union{Tag: "double_list", Value: t}
		case []bool:
			dbMap[attributeName] = duckdb.Union{Tag: "boolean_list", Value: t}
		case []any:
			derivedTag, err := getListTypeTag(t)
			if err != nil {
				strList := make([]string, len(t))
				for i, v := range t {
					strList[i] = fmt.Sprintf("%v", v)
				}
				dbMap[attributeName] = duckdb.Union{Tag: "str_list", Value: strList}
				log.Printf(errors.WarnUnsupportedListAttribute, attributeName, err)
			} else {
				dbMap[attributeName] = duckdb.Union{Tag: derivedTag, Value: t}
			}
		default:
			dbMap[attributeName] = duckdb.Union{Tag: "str", Value: fmt.Sprintf("%v", attributeValue)}
			log.Printf(errors.WarnUnsupportedAttributeType, attributeName, t, attributeValue)
		}
	}
	return dbMap, nil
}

// Scan converts a DuckDB Map type to a map of attributes.
func (attrs *Attributes) Scan(src any) error {
	if src == nil {
		*attrs = Attributes{}
		return nil
	}

	// Check if src reflects to a map type
	rv := reflect.ValueOf(src)
	if rv.Kind() != reflect.Map {
		return fmt.Errorf("Attributes: cannot scan from %T", src)
	}

	attributes := Attributes{}
	for _, key := range rv.MapKeys() {
		if name, ok := key.Interface().(string); ok {
			union := rv.MapIndex(key).Interface()
			if unionValue, ok := union.(duckdb.Union); ok {
				attributes[name] = unionValue.Value
			}
		}
	}

	*attrs = attributes
	return nil
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
		return tag, fmt.Errorf(errors.ErrNilFirstElement)
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
		return tag, fmt.Errorf(errors.ErrUnsupportedListType, list[0])
	}
}

// validateUniformList validates the homogeneity of a list attribute to conform with OTel spec.
func validateUniformList[T any](list []any) error {
	var zero T

	for _, item := range list {
		if item == nil {
			return fmt.Errorf(errors.ErrNilValue)
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
					return fmt.Errorf(errors.ErrUint64Overflow, val)
				}
			default:
				return fmt.Errorf(errors.ErrIncompatibleIntType, item)
			}
		case float64:
			switch item.(type) {
			case float32, float64:
				// All float types can be converted to float64
			default:
				return fmt.Errorf(errors.ErrIncompatibleFloatType, item)
			}
		default:
			// For all other types, use standard type assertion
			if _, ok := item.(T); !ok {
				return fmt.Errorf(errors.ErrIncompatibleType, item)
			}
		}
	}
	return nil
}
