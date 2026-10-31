// Package arrowingest builds whole received batches with shared Arrow contexts.
package arrowingest

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/plog"
)

func field(name string, dt arrow.DataType) arrow.Field {
	return arrow.Field{Name: name, Type: dt, Nullable: true}
}

var text = arrow.BinaryTypes.String
var blob = arrow.BinaryTypes.Binary
var u32 = arrow.PrimitiveTypes.Uint32
var u64 = arrow.PrimitiveTypes.Uint64
var resource = arrow.StructOf(field("id", text), field("payload_id", text), field("schema_url", text), field("attributes", blob), field("dropped_attributes_count", u32))
var scope = arrow.StructOf(field("id", text), field("name", text), field("version", text), field("schema_url", text), field("attributes", blob), field("dropped_attributes_count", u32))
var schema = arrow.NewSchema([]arrow.Field{
	field("id", text), field("timestamp", u64), field("observed_timestamp", u64),
	field("trace_id", text), field("span_id", u64), field("severity_text", text),
	field("severity_number", arrow.PrimitiveTypes.Int32), field("body", blob),
	field("resource", resource), field("scope", scope), field("attributes", blob),
	field("dropped_attributes_count", u32), field("flags", u32), field("event_name", text), field("service_name", text),
}, nil)

type Appender struct {
	ctx             context.Context
	schema          *arrow.Schema
	query           string
	resourceColumn  int
	scopeColumn     int
	conn            driver.Conn
	b               *array.RecordBuilder
	n               int
	closed          bool
	resourceValues  []any
	scopeValues     []any
	shared          bool
	resourceBuilder *array.StructBuilder
	scopeBuilder    *array.StructBuilder
	resourceIndices *array.Int32Builder
	scopeIndices    *array.Int32Builder
	resourceIDs     map[string]int32
	scopeIDs        map[string]int32
	resourceIndex   int32
	scopeIndex      int32
	contextBound    bool
	resourceKey     string
	scopeKey        string
}

// SetContext converts shared context identities once per scope, outside the row loop.
func (a *Appender) SetContext(r, s map[string]any) {
	if !a.shared {
		a.shared = true
		a.resourceBuilder = array.NewStructBuilder(arrowbridge.Allocator(), resource)
		a.scopeBuilder = array.NewStructBuilder(arrowbridge.Allocator(), scope)
		a.resourceIndices = array.NewInt32Builder(arrowbridge.Allocator())
		a.scopeIndices = array.NewInt32Builder(arrowbridge.Allocator())
		a.resourceIDs = map[string]int32{}
		a.scopeIDs = map[string]int32{}
	}
	a.resourceValues = []any{uuid.UUID(r["id"].(duckdb.UUID)).String(), uuid.UUID(r["payload_id"].(duckdb.UUID)).String(), r["schema_url"], r["attributes"], r["dropped_attributes_count"]}
	a.scopeValues = []any{uuid.UUID(s["id"].(duckdb.UUID)).String(), s["name"], s["version"], s["schema_url"], s["attributes"], s["dropped_attributes_count"]}
	// Content identities canonicalize NaNs; transport dictionaries also retain
	// the received bytes. Prepare these payload-sized keys once per context.
	a.resourceKey = a.resourceValues[0].(string) + string(a.resourceValues[3].([]byte))
	a.scopeKey = a.scopeValues[0].(string) + string(a.scopeValues[4].([]byte))
	a.bindContext()
}

// AppendLog writes pdata scalar columns directly, without a variadic driver row.
// The complex-value encoder is the same direct-pdata encoder as production.
func (a *Appender) AppendLog(r plog.LogRecord, id uuid.UUID, body, attrs []byte, service string) error {
	if a.closed {
		return fmt.Errorf("closed Arrow appender")
	}
	a.b.Field(0).(*array.StringBuilder).Append(id.String())
	a.b.Field(1).(*array.Uint64Builder).Append(uint64(r.Timestamp()))
	a.b.Field(2).(*array.Uint64Builder).Append(uint64(r.ObservedTimestamp()))
	trace := a.b.Field(3).(*array.StringBuilder)
	if r.TraceID().IsEmpty() {
		trace.AppendNull()
	} else {
		trace.Append(uuid.UUID(r.TraceID()).String())
	}
	span := a.b.Field(4).(*array.Uint64Builder)
	if r.SpanID().IsEmpty() {
		span.AppendNull()
	} else {
		sid := r.SpanID()
		var n uint64
		for _, x := range sid {
			n = n<<8 | uint64(x)
		}
		span.Append(n)
	}
	a.b.Field(5).(*array.StringBuilder).Append(r.SeverityText())
	a.b.Field(6).(*array.Int32Builder).Append(int32(r.SeverityNumber()))
	a.b.Field(7).(*array.BinaryBuilder).Append(body)
	a.appendContext()
	a.b.Field(10).(*array.BinaryBuilder).Append(attrs)
	a.b.Field(11).(*array.Uint32Builder).Append(r.DroppedAttributesCount())
	a.b.Field(12).(*array.Uint32Builder).Append(uint32(r.Flags()))
	a.b.Field(13).(*array.StringBuilder).Append(r.EventName())
	a.b.Field(14).(*array.StringBuilder).Append(service)
	a.n++
	return nil
}

func (a *Appender) appendContext() {
	if !a.contextBound {
		a.bindContext()
	}
	a.resourceIndices.Append(a.resourceIndex)
	a.scopeIndices.Append(a.scopeIndex)
}

func (a *Appender) bindContext() {
	ri, ok := a.resourceIDs[a.resourceKey]
	if !ok {
		ri = int32(len(a.resourceIDs))
		a.resourceIDs[a.resourceKey] = ri
		rb := a.resourceBuilder
		rb.Append(true)
		rb.FieldBuilder(0).(*array.StringBuilder).Append(a.resourceValues[0].(string))
		rb.FieldBuilder(1).(*array.StringBuilder).Append(a.resourceValues[1].(string))
		rb.FieldBuilder(2).(*array.StringBuilder).Append(a.resourceValues[2].(string))
		rb.FieldBuilder(3).(*array.BinaryBuilder).Append(a.resourceValues[3].([]byte))
		rb.FieldBuilder(4).(*array.Uint32Builder).Append(a.resourceValues[4].(uint32))
	}
	si, ok := a.scopeIDs[a.scopeKey]
	if !ok {
		si = int32(len(a.scopeIDs))
		a.scopeIDs[a.scopeKey] = si
		sb := a.scopeBuilder
		sb.Append(true)
		for i := range 4 {
			sb.FieldBuilder(i).(*array.StringBuilder).Append(a.scopeValues[i].(string))
		}
		sb.FieldBuilder(4).(*array.BinaryBuilder).Append(a.scopeValues[4].([]byte))
		sb.FieldBuilder(5).(*array.Uint32Builder).Append(a.scopeValues[5].(uint32))
	}
	a.resourceIndex, a.scopeIndex = ri, si
	a.contextBound = true
}

func NewAppenders(ctx context.Context, conn driver.Conn, tables []string) (map[string]*Appender, error) {
	out := make(map[string]*Appender, len(tables))
	for _, table := range tables {
		a, err := NewTableAppender(ctx, conn, table, "")
		if err != nil {
			CloseAppenders(out, tables)
			return nil, err
		}
		out[table] = a
	}
	return out, nil
}

func (a *Appender) Flush() error {
	if a.n == 0 {
		return nil
	}
	var record arrow.RecordBatch
	if a.shared {
		fields := a.schema.Fields()
		columns := make([]arrow.Array, len(fields))
		for i := range fields {
			if i == a.resourceColumn || i == a.scopeColumn {
				continue
			}
			columns[i] = a.b.Field(i).NewArray()
		}
		for _, i := range []int{a.resourceColumn, a.scopeColumn} {
			var indices, values arrow.Array
			if i == a.resourceColumn {
				indices, values = a.resourceIndices.NewArray(), a.resourceBuilder.NewArray()
			} else {
				indices, values = a.scopeIndices.NewArray(), a.scopeBuilder.NewArray()
			}
			contextFields := values.DataType().(*arrow.StructType).Fields()
			children := make([]arrow.Array, len(contextFields))
			names := make([]string, len(contextFields))
			for j, f := range contextFields {
				dt := &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: f.Type}
				children[j] = array.NewDictionaryArray(dt, indices, values.(*array.Struct).Field(j))
				names[j] = f.Name
			}
			structArray, err := array.NewStructArray(children, names)
			for _, c := range children {
				c.Release()
			}
			if err != nil {
				indices.Release()
				values.Release()
				return err
			}
			fields[i].Type = structArray.DataType()
			columns[i] = structArray
			indices.Release()
			values.Release()
		}
		record = array.NewRecordBatch(arrow.NewSchema(fields, nil), columns, int64(a.n))
		for _, c := range columns {
			c.Release()
		}
		clear(a.resourceIDs)
		clear(a.scopeIDs)
		a.contextBound = false
	} else {
		record = a.b.NewRecordBatch()
	}
	a.n = 0
	defer record.Release()
	return arrowbridge.Insert(a.ctx, a.conn, a.query, record)
}

func (a *Appender) Close() error {
	if a.closed {
		return nil
	}
	defer a.Discard()
	return a.Flush()
}

// Discard releases a partially built batch without submitting it. Error paths
// must not try to construct a record from incomplete nested columns.
func (a *Appender) Discard() {
	if a.closed {
		return
	}
	a.closed = true
	a.b.Release()
	if a.shared {
		a.resourceBuilder.Release()
		a.scopeBuilder.Release()
		a.resourceIndices.Release()
		a.scopeIndices.Release()
	}
}

func CloseAppenders(as map[string]*Appender, tables []string) error {
	var err error
	for i := len(tables) - 1; i >= 0; i-- {
		if a := as[tables[i]]; a != nil {
			err = errors.Join(err, a.Close())
		}
	}
	return err
}
