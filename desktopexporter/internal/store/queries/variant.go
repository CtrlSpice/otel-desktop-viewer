package queries

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// RegisterFunctions installs native-value encoders once per database.
// DuckDB's Go driver cannot scan VARIANT directly. Its standard Parquet VARIANT
// projection exposes typed binary data, which Arrow decodes without a JSON
// round-trip (including exact integers, bytes, and IEEE-754 payload bits).
func RegisterFunctions(ctx context.Context, db *sql.DB) error {
	blob, err := duckdb.NewTypeInfo(duckdb.TYPE_BLOB)
	if err != nil {
		return err
	}
	text, err := duckdb.NewTypeInfo(duckdb.TYPE_VARCHAR)
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	for _, projection := range []struct {
		name   string
		encode func(pcommon.Value) ([]byte, error)
	}{
		{"otel_wire_json", util.EncodeValue},
		{"otel_otlp_json", encodeOTLPValue},
		{"otel_native_text", func(v pcommon.Value) ([]byte, error) { return []byte(v.AsString()), nil }},
	} {
		err = duckdb.RegisterScalarUDF(conn, projection.name, &variantJSON{
			config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: text},
			encode: projection.encode,
		})
		if err != nil {
			break
		}
	}
	if err == nil {
		err = duckdb.RegisterScalarUDF(conn, "otel_native_object_field", &variantObjectField{
			config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob, text}, ResultTypeInfo: blob},
		})
	}
	if err == nil {
		err = duckdb.RegisterScalarUDF(conn, "otel_native_preview", &variantPreview{
			config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: text},
		})
	}
	if err == nil {
		err = duckdb.RegisterScalarUDF(conn, "otel_native_column_preview", &variantPreview{
			config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: text},
			column: true,
		})
	}
	if err == nil {
		err = duckdb.RegisterScalarUDF(conn, "otel_home_value_text", &variantHomeText{
			config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: text},
		})
	}
	if err == nil {
		var keys duckdb.TypeInfo
		keys, err = duckdb.NewListInfo(text)
		if err == nil {
			err = duckdb.RegisterScalarUDF(conn, "otel_metric_label_columns", &metricLabelColumns{
				config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{text}, ResultTypeInfo: keys},
			})
		}
	}
	if err == nil {
		var result duckdb.TypeInfo
		result, err = metricLabelInfoType(text)
		if err == nil {
			err = duckdb.RegisterScalarUDF(conn, "otel_metric_label_info", &metricLabelInfo{
				config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: result},
			})
		}
	}
	if err == nil {
		var result duckdb.TypeInfo
		result, err = attributePreviewType(text)
		if err == nil {
			err = duckdb.RegisterScalarUDF(conn, "otel_native_attribute_previews", &variantAttributePreviews{
				config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob, text}, ResultTypeInfo: result},
			})
		}
	}
	return errors.Join(err, conn.Close())
}

// DuckDB's variant_extract requires a constant field name. Discovery enumerates
// received keys, so use the same native binary reader as the output encoders for
// dynamic object access. Copy the child's original bytes without JSON conversion
// or rebuilding its scalar types. Array children use DuckDB's native list cast.
type variantObjectField struct{ config duckdb.ScalarFuncConfig }

func (f *variantObjectField) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *variantObjectField) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		v, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		object, ok := v.Value().(parquetvariant.ObjectValue)
		if !ok {
			return nil, fmt.Errorf("native field access requires an object, got %v", v.Type())
		}
		field, err := object.ValueByKey(args[2].(string))
		if err != nil {
			return nil, err
		}
		metadata, value := field.Value.Metadata().Bytes(), field.Value.Bytes()
		out := make([]byte, 0, len(metadata)+len(value))
		out = append(out, metadata...)
		return append(out, value...), nil
	}}
}

type variantJSON struct {
	config duckdb.ScalarFuncConfig
	encode func(pcommon.Value) ([]byte, error)
}

func (f *variantJSON) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *variantJSON) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		v, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		out := pcommon.NewValueEmpty()
		if err := decodeVariant(v, out); err != nil {
			return nil, err
		}
		encoded, err := f.encode(out)
		return string(encoded), err
	}}
}

func decodeVariant(v parquetvariant.Value, out pcommon.Value) error {
	switch x := v.Value().(type) {
	case nil:
	case string:
		out.SetStr(x)
	case bool:
		out.SetBool(x)
	case int8:
		out.SetInt(int64(x))
	case int16:
		out.SetInt(int64(x))
	case int32:
		out.SetInt(int64(x))
	case int64:
		out.SetInt(x)
	case float64:
		out.SetDouble(x)
	case []byte:
		out.SetEmptyBytes().FromRaw(x)
	case parquetvariant.ArrayValue:
		s := out.SetEmptySlice()
		for i := uint32(0); i < x.Len(); i++ {
			child, err := x.Value(i)
			if err != nil {
				return err
			}
			if err := decodeVariant(child, s.AppendEmpty()); err != nil {
				return err
			}
		}
	case parquetvariant.ObjectValue:
		m := out.SetEmptyMap()
		for key, child := range x.Values() {
			if err := decodeVariant(child, m.PutEmpty(key)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("non-OTel VARIANT type %v", v.Type())
	}
	return nil
}
