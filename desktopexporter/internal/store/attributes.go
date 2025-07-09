package store

import (
	"fmt"
	"log"
	"math"

	"github.com/marcboeker/go-duckdb/v2"
)

// An Attribute is a key-value pair, which MUST have the following properties:
// - The attribute key MUST be a non-null and non-empty string.
// - Case sensitivity of keys is preserved. Keys that differ in casing are treated as distinct keys.
// - The attribute value is either:
//   - A primitive type: string, boolean, double precision floating point (IEEE 754-1985) or signed 64 bit integer.
//   - An array of primitive type values. The array MUST be homogeneous, i.e., it MUST NOT contain values of different types.

// toDbAttributes converts a map of attributes to a DuckDB Map type.
// For uint64 values, if they exceed math.MaxInt64, they are converted to strings.
// For []uint64 values, if any value exceeds math.MaxInt64, the entire slice is converted to []string.
func toDbAttributes(attributes map[string]any) duckdb.Map {
	dbMap := duckdb.Map{}

	for attributeName, attributeValue := range attributes {
		switch t := attributeValue.(type) {
		case string:
			dbMap[attributeName] = duckdb.Union{Tag: "str", Value: t}
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
			dbMap[attributeName] = duckdb.Union{Tag: "bigint", Value: t}
		case uint64:
			value, hasOverflow := stringifyOnOverflow(attributeName, t)
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
			value, hasOverflow := stringifyOnOverflow(attributeName, t...)
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
				log.Printf(WarnUnsupportedListAttribute, attributeName, err)
			} else {
				dbMap[attributeName] = duckdb.Union{Tag: derivedTag, Value: t}
			}
		default:
			dbMap[attributeName] = duckdb.Union{Tag: "str", Value: fmt.Sprintf("%v", attributeValue)}
			log.Printf(WarnUnsupportedAttributeType, attributeName, t, attributeValue)
		}
	}

	return dbMap
}

func fromDbAttributes(rawAttributes map[string]duckdb.Union) map[string]any {
	attributes := map[string]any{}

	for attrName, union := range rawAttributes {
		attributes[attrName] = union.Value
	}

	return attributes
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

// stringifyOnOverflow checks uint64 values for overflow beyond math.MaxInt64.
// If any value exceeds math.MaxInt64:
//   - For single values: returns the string representation and true
//   - For slices: converts all values to strings and returns true
//
// If no overflow occurs:
//   - For single values: returns the int64 value and false
//   - For slices: returns the []int64 values and false
func stringifyOnOverflow(attributeName string, values ...uint64) (any, bool) {
	hasOverflow := false
	for _, val := range values {
		if val > math.MaxInt64 {
			hasOverflow = true
			break
		}
	}

	// No overflow - convert all values to int64
	if !hasOverflow {
		if len(values) == 1 {
			return int64(values[0]), false
		}

		int64List := make([]int64, len(values))
		for i, val := range values {
			int64List[i] = int64(val)
		}
		return int64List, false
	}

	// Has overflow - convert all values to string
	if len(values) == 1 {
		strVal := fmt.Sprintf("%v", values[0])
		log.Printf(WarnUint64Overflow, attributeName, values[0])
		return strVal, true
	}

	strList := make([]string, len(values))
	for i, v := range values {
		strList[i] = fmt.Sprintf("%v", v)
	}
	log.Printf(WarnUint64SliceOverflow, attributeName)
	return strList, true
}
