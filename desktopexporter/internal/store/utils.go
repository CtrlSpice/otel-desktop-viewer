package store

import "reflect"

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
			resultField.Set(sourceField) // Direct copy if types match
		} else if overrideValue, exists := overrides[fieldName]; exists {
			// Apply override if field exists but types don't match
			if resultField.IsValid() && resultField.CanSet() {
				resultField.Set(reflect.ValueOf(overrideValue))
			}
		}
	}

	return result
}
