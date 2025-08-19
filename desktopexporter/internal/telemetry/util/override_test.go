package util

import (
	"reflect"
	"testing"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes"`
	Count      int            `json:"count"`
}

func TestCopyAndOverrideWithTypeConversion(t *testing.T) {
	source := TestStruct{
		Name: "test",
		Attributes: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
		Count: 100,
	}

	// Override Attributes with a duckdb.Map (different type)
	duckdbAttrs := make(duckdb.Map)
	duckdbAttrs["key1"] = duckdb.Union{Value: "value1", Tag: "VARCHAR"}
	duckdbAttrs["key2"] = duckdb.Union{Value: 42, Tag: "BIGINT"}
	duckdbAttrs["key3"] = duckdb.Union{Value: true, Tag: "BOOLEAN"}
	duckdbAttrs["key4"] = duckdb.Union{Value: 3.14, Tag: "DOUBLE"}

	result := CopyAndOverride(source, map[string]any{
		"Attributes": duckdbAttrs,
	})

	// Result is now a struct with the same fields but Attributes is duckdb.Map
	// We need to use reflection to access the fields since the type is dynamic
	resultValue := reflect.ValueOf(result)

	// Check Name field
	nameField := resultValue.FieldByName("Name")
	assert.True(t, nameField.IsValid())
	assert.Equal(t, "test", nameField.Interface())

	// Check Count field
	countField := resultValue.FieldByName("Count")
	assert.True(t, countField.IsValid())
	assert.Equal(t, 100, countField.Interface())

	// Check Attributes field - should now be duckdb.Map type
	attrsField := resultValue.FieldByName("Attributes")
	assert.True(t, attrsField.IsValid())
	assert.IsType(t, duckdb.Map{}, attrsField.Interface())

	// Verify the converted data
	attrs := attrsField.Interface().(duckdb.Map)
	assert.Len(t, attrs, 4)
	assert.Equal(t, "value1", attrs["key1"].(duckdb.Union).Value)
	assert.Equal(t, "VARCHAR", attrs["key1"].(duckdb.Union).Tag)
	assert.Equal(t, 42, attrs["key2"].(duckdb.Union).Value)
	assert.Equal(t, "BIGINT", attrs["key2"].(duckdb.Union).Tag)
	assert.Equal(t, true, attrs["key3"].(duckdb.Union).Value)
	assert.Equal(t, "BOOLEAN", attrs["key3"].(duckdb.Union).Tag)
	assert.Equal(t, 3.14, attrs["key4"].(duckdb.Union).Value)
	assert.Equal(t, "DOUBLE", attrs["key4"].(duckdb.Union).Tag)
}

func TestCopyAndOverrideWithNoOverrides(t *testing.T) {
	source := TestStruct{
		Name:       "original",
		Attributes: map[string]any{"original": "data"},
		Count:      123,
	}

	result := CopyAndOverride(source, map[string]any{})

	// Result is now a struct with the same fields and types as source
	resultValue := reflect.ValueOf(result)

	// Check Name field
	nameField := resultValue.FieldByName("Name")
	assert.True(t, nameField.IsValid())
	assert.Equal(t, "original", nameField.Interface())

	// Check Count field
	countField := resultValue.FieldByName("Count")
	assert.True(t, countField.IsValid())
	assert.Equal(t, 123, countField.Interface())

	// Check Attributes field - should remain as map[string]any since no override was provided
	attrsField := resultValue.FieldByName("Attributes")
	assert.True(t, attrsField.IsValid())
	assert.IsType(t, map[string]any{}, attrsField.Interface())
}
