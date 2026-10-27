package ingest

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"sort"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ValueNodes copies an OTel value into the SQL otel_value_node[] transport. The
// complete tree occupies one appender cell. No pdata references survive this
// call, and no key or value becomes SQL text. Maps follow the existing last-key-
// wins identity rule; slices retain order and repeated values.
func ValueNodes(value pcommon.Value) ([]any, error) {
	var nodes []any
	var visit func(pcommon.Value, int64, int64, string) error
	visit = func(v pcommon.Value, parent, ordinal int64, key string) error {
		id := int64(len(nodes))
		n := map[string]any{
			"id": id, "parent": parent, "ordinal": ordinal, "key": key,
			"children": int64(0), "string_value": nil, "int_value": nil,
			"double_value": nil, "bool_value": nil, "bytes_value": nil,
		}
		nodes = append(nodes, n)
		switch v.Type() {
		case pcommon.ValueTypeEmpty:
			n["kind"] = "empty"
		case pcommon.ValueTypeStr:
			n["kind"], n["string_value"] = "string", v.Str()
		case pcommon.ValueTypeInt:
			n["kind"], n["int_value"] = "int64", v.Int()
		case pcommon.ValueTypeDouble:
			n["kind"], n["double_value"] = "double", util.CanonicalDouble(v.Double())
		case pcommon.ValueTypeBool:
			n["kind"], n["bool_value"] = "bool", v.Bool()
		case pcommon.ValueTypeBytes:
			b := make([]byte, v.Bytes().Len())
			copy(b, v.Bytes().AsRaw())
			n["kind"], n["bytes_value"] = "bytes", b
		case pcommon.ValueTypeSlice:
			s := v.Slice()
			n["kind"], n["children"] = "array", int64(s.Len())
			for i := 0; i < s.Len(); i++ {
				if err := visit(s.At(i), id, int64(i), ""); err != nil {
					return err
				}
			}
		case pcommon.ValueTypeMap:
			latest := util.LastValues(v.Map())
			keys := make([]string, 0, len(latest))
			for key := range latest {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			n["kind"], n["children"] = "map", int64(len(keys))
			for i, key := range keys {
				if err := visit(latest[key], id, int64(i), key); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported OpenTelemetry value kind %d", v.Type())
		}
		return nil
	}
	if err := visit(value, -1, 0, ""); err != nil {
		return nil, err
	}
	return nodes, nil
}

// NewTypedQueryAppender derives transport types from a fixed SELECT. This keeps
// SQL type definitions authoritative instead of duplicating their fields in Go.
// query and shape must be application constants, never received telemetry.
func NewTypedQueryAppender(conn driver.Conn, query, shape string) (*duckdb.Appender, error) {
	stmt, err := conn.Prepare(shape)
	if err != nil {
		return nil, err
	}
	s, ok := stmt.(*duckdb.Stmt)
	if !ok {
		return nil, errors.Join(fmt.Errorf("query appender requires DuckDB"), stmt.Close())
	}
	// Query the shape only for its column names; it always returns zero rows.
	rows, err := stmt.Query(nil)
	if err != nil {
		return nil, errors.Join(err, stmt.Close())
	}
	names := rows.Columns()
	types := make([]duckdb.TypeInfo, len(names))
	for i := range names {
		types[i], err = s.ColumnTypeInfo(i)
		if err != nil {
			return nil, errors.Join(err, rows.Close(), stmt.Close())
		}
	}
	if err := errors.Join(rows.Close(), stmt.Close()); err != nil {
		return nil, err
	}
	return duckdb.NewQueryAppender(conn, query, "", types, names)
}
