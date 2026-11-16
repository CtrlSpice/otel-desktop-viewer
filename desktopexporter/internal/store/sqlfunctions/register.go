package sqlfunctions

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// Register installs native-value encoders once per database.
// DuckDB's Go driver cannot scan VARIANT directly. Its standard Parquet VARIANT
// projection exposes typed binary data, which Arrow decodes without a JSON
// round-trip (including exact integers, bytes, and IEEE-754 payload bits).
func Register(ctx context.Context, db *sql.DB) error {
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
	if err == nil {
		var result duckdb.TypeInfo
		result, err = metricSortInfoType(text, blob)
		if err == nil {
			err = duckdb.RegisterScalarUDF(conn, "otel_metric_sort_info", &metricSortInfo{
				config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: result},
			})
		}
	}
	if err == nil {
		var boolean duckdb.TypeInfo
		boolean, err = duckdb.NewTypeInfo(duckdb.TYPE_BOOLEAN)
		if err == nil {
			err = duckdb.RegisterScalarUDF(conn, "otel_metric_text_sort_key", &metricTextSortKey{
				config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{text, boolean}, ResultTypeInfo: blob},
			})
		}
	}
	return errors.Join(err, conn.Close())
}
