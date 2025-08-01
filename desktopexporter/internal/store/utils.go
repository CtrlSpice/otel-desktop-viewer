package store

import (
	"reflect"
)

// copyAndOverride is a generic helper function that copies most fields from a struct
// but overrides specified fields with new values/types.
// We use this to transform our telemetry datatypes into a format that makes duckdb happy.
func copyAndOverride[SourceType any, ResultType any](source SourceType, overrides map[string]any) ResultType {
	var result ResultType
	sourceValue := reflect.ValueOf(source)
	resultValue := reflect.ValueOf(&result).Elem()

	// Copy all fields from source to result, with overrides for type mismatches
	for i := 0; i < sourceValue.NumField(); i++ {
		fieldName := sourceValue.Type().Field(i).Name
		sourceField := sourceValue.Field(i)
		resultField := resultValue.FieldByName(fieldName)

		if resultField.IsValid() && resultField.CanSet() {
			// Check if we have an override for this field
			if overrideValue, exists := overrides[fieldName]; exists {
				resultField.Set(reflect.ValueOf(overrideValue))
			} else {
				// Try direct copy, but handle type conversion if needed
				if sourceField.Type().ConvertibleTo(resultField.Type()) {
					resultField.Set(sourceField.Convert(resultField.Type()))
				} else {
					resultField.Set(sourceField) // Direct copy if types match
				}
			}
		}
	}

	return result
}
