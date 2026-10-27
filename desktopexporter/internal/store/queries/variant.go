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

// RegisterFunctions installs the native-value wire encoder once per database.
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
	err = duckdb.RegisterScalarUDF(conn, "otel_wire_json", &variantJSON{
		config: duckdb.ScalarFuncConfig{InputTypeInfos: []duckdb.TypeInfo{blob, blob}, ResultTypeInfo: text},
	})
	return errors.Join(err, conn.Close())
}

type variantJSON struct{ config duckdb.ScalarFuncConfig }

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
		encoded, err := util.EncodeValue(out)
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
