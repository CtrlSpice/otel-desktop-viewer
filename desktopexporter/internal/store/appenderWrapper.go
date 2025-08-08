package store

import (
	"database/sql/driver"
	"reflect"

	"github.com/marcboeker/go-duckdb/v2"
)

type AppenderWrapper struct {
	appender *duckdb.Appender
}

func NewAppenderWrapper(conn driver.Conn, catalog, schema, table string) (*AppenderWrapper, error) {
	appender, err := duckdb.NewAppender(conn, catalog, schema, table)
	if err != nil {
		return nil, err
	}
	return &AppenderWrapper{appender: appender}, nil
}

// convertValue recursively converts a value, calling Value() on any driver.Valuer types found
func convertValue(arg any) (any, error) {
	if arg == nil {
		return nil, nil
	}

	// Check if the value itself implements driver.Valuer
	if valuer, ok := arg.(driver.Valuer); ok {
		val, err := valuer.Value()
		if err != nil {
			return nil, err
		}
		// Recursively process the result of Value() in case it contains other driver.Valuer types
		return convertValue(val)
	}

	v := reflect.ValueOf(arg)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		if v.IsNil() {
			return nil, nil
		}
		result := make([]any, v.Len())
		for i := range v.Len() {
			element := v.Index(i).Interface()
			// Check if the element implements driver.Valuer
			if valuer, ok := element.(driver.Valuer); ok {
				val, err := valuer.Value()
				if err != nil {
					return nil, err
				}
				result[i] = val
			} else {
				// Otherwise process recursively
				converted, err := convertValue(element)
				if err != nil {
					return nil, err
				}
				result[i] = converted
			}
		}
		return result, nil

	case reflect.Struct:
		// Handle duckdb.Union types specially - process Value field recursively
		if union, ok := arg.(duckdb.Union); ok {
			convertedValue, err := convertValue(union.Value)
			if err != nil {
				return nil, err
			}
			return duckdb.Union{Tag: union.Tag, Value: convertedValue}, nil
		}

		// Create a new struct with the same shape but converted fields
		t := v.Type()
		fields := make([]reflect.StructField, 0, v.NumField())
		values := make([]any, 0, v.NumField())

		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := t.Field(i)

			// Skip unexported fields
			if !field.CanInterface() {
				continue
			}

			// Check if the field implements driver.Valuer
			fieldValue := field.Interface()
			var converted any
			var err error

			if valuer, ok := fieldValue.(driver.Valuer); ok {
				converted, err = valuer.Value()
				if err != nil {
					return nil, err
				}
			} else {
				// Process the field recursively
				converted, err = convertValue(fieldValue)
				if err != nil {
					return nil, err
				}
			}

			// Create struct field with original name and converted type
			convertedType := reflect.TypeOf(converted)
			fields = append(fields, reflect.StructField{
				Name: fieldType.Name,
				Type: convertedType,
				Tag:  fieldType.Tag,
			})
			values = append(values, converted)
		}

		// Create the new struct type
		newStructType := reflect.StructOf(fields)
		resultValue := reflect.New(newStructType).Elem()

		// Set the values
		for i, value := range values {
			resultValue.Field(i).Set(reflect.ValueOf(value))
		}

		return resultValue.Interface(), nil

	case reflect.Ptr:
		if v.IsNil() {
			return nil, nil
		}
		return convertValue(v.Elem().Interface())

	default:
		// For primitive types, return as-is
		return arg, nil
	}
}

// AppendRow converts args to driver.Value, calling Value() on types that implement driver.Valuer
func (aw *AppenderWrapper) AppendRow(args ...any) error {
	convertedArgs := make([]driver.Value, len(args))

	for i, arg := range args {
		converted, err := convertValue(arg)
		if err != nil {
			return err
		}

		// Handle nil case properly
		if converted == nil {
			convertedArgs[i] = nil
		} else {
			convertedArgs[i] = converted.(driver.Value)
		}
	}
	return aw.appender.AppendRow(convertedArgs...)
}

func (aw *AppenderWrapper) Flush() error {
	return aw.appender.Flush()
}

func (aw *AppenderWrapper) Close() error {
	return aw.appender.Close()
}
