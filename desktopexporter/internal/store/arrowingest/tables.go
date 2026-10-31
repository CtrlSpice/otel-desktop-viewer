package arrowingest

import (
	"context"
	"database/sql/driver"
	"fmt"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"strings"
)

var i32 = arrow.PrimitiveTypes.Int32
var i64 = arrow.PrimitiveTypes.Int64
var f64 = arrow.PrimitiveTypes.Float64

type tableShape struct {
	fields          []arrow.Field
	variants, uuids []string
}

var shapes = map[string]tableShape{
	"logs": {schema.Fields(), []string{"body", "attributes", "resource.attributes", "scope.attributes"}, []string{"id", "trace_id"}},
	"spans": {[]arrow.Field{
		field("trace_id", text), field("trace_state", text), field("span_id", u64), field("parent_span_id", u64), field("flags", u32), field("name", text), field("kind", i32), field("start_time", u64), field("end_time", u64), field("resource", resource), field("scope", scope), field("attributes", blob), field("dropped_attributes_count", u32), field("dropped_events_count", u32), field("dropped_links_count", u32), field("status_code", i32), field("status_message", text), field("service_name", text),
	}, []string{"attributes", "resource.attributes", "scope.attributes"}, []string{"trace_id"}},
	"events": {[]arrow.Field{
		field("id", text), field("trace_id", text), field("span_id", u64), field("name", text), field("timestamp", u64), field("attributes", blob), field("dropped_attributes_count", u32), field("ordinal", u32),
	}, []string{"attributes"}, []string{"id", "trace_id"}},
	"links": {[]arrow.Field{
		field("id", text), field("trace_id", text), field("span_id", u64), field("linked_trace_id", text), field("linked_span_id", u64), field("trace_state", text), field("attributes", blob), field("dropped_attributes_count", u32), field("flags", u32), field("ordinal", u32),
	}, []string{"attributes"}, []string{"id", "trace_id", "linked_trace_id"}},
	"exemplars": {[]arrow.Field{
		field("id", text), field("metric_datapoint_id", text), field("timestamp", u64), field("double_value", f64), field("int_value", i64), field("trace_id", text), field("span_id", u64), field("attributes", blob),
	}, []string{"attributes"}, []string{"id", "metric_datapoint_id", "trace_id"}},
	"metrics": {[]arrow.Field{
		field("id", text), field("resource_id", text), field("resource_payload_id", text), field("scope_id", text), field("name", text), field("description", text), field("unit", text), field("metadata", blob), field("metric_type", text), field("aggregation_temporality", i32), field("is_monotonic", arrow.FixedWidthTypes.Boolean), field("service_name", text), field("resource", resource), field("scope", scope),
	}, []string{"metadata", "resource.attributes", "scope.attributes"}, []string{"id", "resource_id", "resource_payload_id", "scope_id"}},
}

func NewTableAppender(ctx context.Context, conn driver.Conn, table, query string) (*Appender, error) {
	shape, ok := shapes[table]
	if !ok {
		return nil, fmt.Errorf("unsupported Arrow table %s", table)
	}
	if query == "" {
		query = ingest.OwnedAppendQuery(table, shape.variants)
	}
	replacements := make([]string, len(shape.uuids))
	for i, name := range shape.uuids {
		replacements[i] = name + "::uuid as " + name
	}
	query = strings.Replace(query, "from appended_data", "from (select * replace("+strings.Join(replacements, ",")+") from appended_data) input", 1)
	s := arrow.NewSchema(shape.fields, nil)
	a := &Appender{ctx: ctx, conn: conn, schema: s, query: query, b: array.NewRecordBuilder(arrowbridge.Allocator(), s), resourceColumn: -1, scopeColumn: -1}
	for i, f := range shape.fields {
		if f.Name == "resource" {
			a.resourceColumn = i
		}
		if f.Name == "scope" {
			a.scopeColumn = i
		}
	}
	return a, nil
}

// NewCustomAppender builds nested owning rows using an application-owned schema.
func NewCustomAppender(ctx context.Context, conn driver.Conn, fields []arrow.Field, query string) *Appender {
	s := arrow.NewSchema(fields, nil)
	a := &Appender{ctx: ctx, conn: conn, schema: s, query: query, b: array.NewRecordBuilder(arrowbridge.Allocator(), s), resourceColumn: -1, scopeColumn: -1}
	for i, f := range fields {
		if f.Name == "resource" {
			a.resourceColumn = i
		}
		if f.Name == "scope" {
			a.scopeColumn = i
		}
	}
	return a
}

func TableFields(table string) []arrow.Field {
	return append([]arrow.Field(nil), shapes[table].fields...)
}
func (a *Appender) Builder() *array.RecordBuilder { return a.b }
func (a *Appender) FinishParent()                 { a.appendContext(); a.n++ }
