package ingest

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"sort"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ValueBytes encodes pdata directly as Parquet Variant metadata followed by its
// value bytes. The returned BLOB owns its storage and contains no pdata references.
// SQL decodes it into one self-contained native VARIANT, without reconstructing
// nodes or inferring scalar types from JSON.
func ValueBytes(value pcommon.Value) ([]byte, error) {
	var b pv.Builder
	if err := appendValue(&b, value); err != nil {
		return nil, err
	}
	return finishValue(&b)
}

// AttributeBytes encodes an owner's complete map without copying it into another
// pdata value. Duplicate keys use the last received value, as identity hashing does.
func AttributeBytes(attributes pcommon.Map) ([]byte, error) {
	var b pv.Builder
	if err := appendMap(&b, attributes); err != nil {
		return nil, err
	}
	return finishValue(&b)
}

func finishValue(b *pv.Builder) ([]byte, error) {
	v, err := b.Build()
	if err != nil {
		return nil, err
	}
	// Build's slices belong to the builder. Copy both before any reset or reuse.
	out := make([]byte, 0, len(v.Metadata().Bytes())+len(v.Bytes()))
	out = append(out, v.Metadata().Bytes()...)
	return append(out, v.Bytes()...), nil
}

func appendMap(b *pv.Builder, attributes pcommon.Map) error {
	start := b.Offset()
	latest := util.LastValues(attributes)
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]pv.FieldEntry, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, b.NextField(start, key))
		if err := appendValue(b, latest[key]); err != nil {
			return err
		}
	}
	return b.FinishObject(start, fields)
}

func appendValue(b *pv.Builder, v pcommon.Value) error {
	switch v.Type() {
	case pcommon.ValueTypeEmpty:
		return b.AppendNull()
	case pcommon.ValueTypeStr:
		return b.AppendString(v.Str())
	case pcommon.ValueTypeInt:
		return b.AppendInt(v.Int())
	case pcommon.ValueTypeDouble:
		return b.AppendFloat64(util.CanonicalDouble(v.Double()))
	case pcommon.ValueTypeBool:
		return b.AppendBool(v.Bool())
	case pcommon.ValueTypeBytes:
		return b.AppendBinary(v.Bytes().AsRaw())
	case pcommon.ValueTypeMap:
		return appendMap(b, v.Map())
	case pcommon.ValueTypeSlice:
		start := b.Offset()
		offsets := make([]int, 0, v.Slice().Len())
		for _, child := range v.Slice().All() {
			offsets = append(offsets, b.NextElement(start))
			if err := appendValue(b, child); err != nil {
				return err
			}
		}
		return b.FinishArray(start, offsets)
	default:
		return fmt.Errorf("unsupported OpenTelemetry value kind %d", v.Type())
	}
}

// ResourceInput copies the received resource and its wrapper into one appender
// cell. The identities are grouping keys, not references to another table.
func ResourceInput(resource pcommon.Resource, schemaURL string) (map[string]any, error) {
	attributes, err := AttributeBytes(resource.Attributes())
	if err != nil {
		return nil, err
	}
	ids, err := AttributeIdentities(resource.Attributes())
	if err != nil {
		return nil, err
	}
	payloadID := ResourcePayloadID(ids, resource.DroppedAttributesCount())
	return map[string]any{
		"id": ResourceID(payloadID, schemaURL), "payload_id": payloadID,
		"schema_url": schemaURL, "attributes": attributes,
		"dropped_attributes_count": resource.DroppedAttributesCount(),
	}, nil
}

// ScopeInput copies the received instrumentation scope and its wrapper.
func ScopeInput(scope pcommon.InstrumentationScope, schemaURL string) (map[string]any, error) {
	attributes, err := AttributeBytes(scope.Attributes())
	if err != nil {
		return nil, err
	}
	ids, err := AttributeIdentities(scope.Attributes())
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":   ScopeID(scope.Name(), scope.Version(), schemaURL, ids, scope.DroppedAttributesCount()),
		"name": scope.Name(), "version": scope.Version(), "schema_url": schemaURL,
		"attributes": attributes, "dropped_attributes_count": scope.DroppedAttributesCount(),
	}, nil
}

// AttributeIdentities preserves existing content identities without constructing
// dictionary rows. These hashes are used only to derive owner and series keys.
func AttributeIdentities(attributes pcommon.Map) ([]duckdb.UUID, error) {
	latest := util.LastValues(attributes)
	ids := make([]duckdb.UUID, 0, len(latest))
	for key, value := range latest {
		encoded, err := util.EncodeValue(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, AttributeID(key, string(encoded)))
	}
	sort.Slice(ids, func(i, j int) bool { return uuidLess(ids[i], ids[j]) })
	return ids, nil
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
