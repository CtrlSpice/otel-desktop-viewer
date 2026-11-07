package queries

import (
	"database/sql/driver"
	"fmt"

	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
)

// Attribute previews preserve repeated root keys without decoding their descendants.
type variantAttributePreviews struct{ config duckdb.ScalarFuncConfig }

func (f *variantAttributePreviews) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *variantAttributePreviews) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		value, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		object, ok := value.Value().(parquetvariant.ObjectValue)
		if !ok {
			return nil, fmt.Errorf("attribute previews require an object")
		}
		result := []any{}
		for i := uint32(0); i < object.NumElements(); i++ {
			field, err := object.FieldAt(i)
			if err != nil {
				return nil, err
			}
			if field.Key != args[2].(string) {
				continue
			}
			preview, err := previewVariant(field.Value, true, true)
			if err != nil {
				return nil, err
			}
			kind := "empty"
			switch field.Value.Value().(type) {
			case string:
				kind = "string"
			case []byte:
				kind = "bytes"
			case bool:
				kind = "bool"
			case int8, int16, int32, int64:
				kind = "int64"
			case float64:
				kind = "double"
			case parquetvariant.ArrayValue:
				kind = "array"
			case parquetvariant.ObjectValue:
				kind = "map"
			}
			result = append(result, map[string]any{"valueType": kind, "preview": preview})
		}
		return result, nil
	}}
}

func attributePreviewType(text duckdb.TypeInfo) (duckdb.TypeInfo, error) {
	kind, err := duckdb.NewStructEntry(text, "valueType")
	if err != nil {
		return nil, err
	}
	preview, err := duckdb.NewStructEntry(text, "preview")
	if err != nil {
		return nil, err
	}
	entry, err := duckdb.NewStructInfo(kind, preview)
	if err != nil {
		return nil, err
	}
	return duckdb.NewListInfo(entry)
}
