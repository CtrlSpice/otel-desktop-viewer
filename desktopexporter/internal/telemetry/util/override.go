package util

import (
	"reflect"
)

// CopyAndOverride takes a source struct, applies overrides to specified fields, and returns the result
func CopyAndOverride[SourceType any](source SourceType, overrides map[string]any) any {
	sourceValue := reflect.ValueOf(source)
	sourceType := sourceValue.Type()

	// Build the new struct type with overridden field types
	var fields []reflect.StructField

	for i := 0; i < sourceType.NumField(); i++ {
		field := sourceType.Field(i)
		fieldName := field.Name

		// Check if we have an override for this field
		if overrideValue, exists := overrides[fieldName]; exists {
			// Use the type of the override value
			fields = append(fields, reflect.StructField{
				Name: fieldName,
				Type: reflect.TypeOf(overrideValue),
				Tag:  field.Tag,
			})
		} else {
			// Use the original field type
			fields = append(fields, field)
		}
	}

	// Create the new struct type
	newStructType := reflect.StructOf(fields)

	// Create a new instance of the new struct type
	resultValue := reflect.New(newStructType).Elem()

	// Copy values from source to result
	for i := 0; i < sourceType.NumField(); i++ {
		fieldName := sourceType.Field(i).Name
		sourceField := sourceValue.Field(i)
		resultField := resultValue.Field(i)

		// Check if we have an override for this field
		if overrideValue, exists := overrides[fieldName]; exists {
			resultField.Set(reflect.ValueOf(overrideValue))
		} else {
			// Copy the field as-is
			resultField.Set(sourceField)
		}
	}

	return resultValue.Interface()
}
