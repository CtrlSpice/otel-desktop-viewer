package nested

import (
	"context"
	"database/sql/driver"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
	"github.com/apache/arrow-go/v18/arrow"
	"strings"
)

func field(n string, t arrow.DataType) arrow.Field {
	return arrow.Field{Name: n, Type: t, Nullable: true}
}

var text = arrow.BinaryTypes.String
var binaryType = arrow.BinaryTypes.Binary
var u64 = arrow.PrimitiveTypes.Uint64
var u32 = arrow.PrimitiveTypes.Uint32
var i32 = arrow.PrimitiveTypes.Int32
var i64 = arrow.PrimitiveTypes.Int64
var f64 = arrow.PrimitiveTypes.Float64

func selected(table string, names ...string) []arrow.Field {
	var out []arrow.Field
	for _, name := range names {
		for _, f := range arrowingest.TableFields(table) {
			if f.Name == name {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

var eventFields = selected("events", "id", "name", "timestamp", "attributes", "dropped_attributes_count", "ordinal")
var linkFields = selected("links", "id", "linked_trace_id", "linked_span_id", "trace_state", "attributes", "dropped_attributes_count", "flags", "ordinal")
var exemplarFields = selected("exemplars", "id", "timestamp", "double_value", "int_value", "trace_id", "span_id", "attributes")
var pointFields = []arrow.Field{
	field("id", text), field("timestamp", u64), field("start_time", u64), field("flags", u32), field("double_value", f64), field("int_value", i64), field("value_type", text), field("count", u64), field("sum", f64), field("min", f64), field("max", f64), field("bucket_counts", arrow.ListOf(u64)), field("explicit_bounds", arrow.ListOf(f64)), field("scale", i32), field("zero_count", u64), field("zero_threshold", f64), field("positive_bucket_offset", i32), field("positive_bucket_counts", arrow.ListOf(u64)), field("negative_bucket_offset", i32), field("negative_bucket_counts", arrow.ListOf(u64)), field("attributes", binaryType), field("attribute_key", text), field("exemplars", arrow.ListOf(arrow.StructOf(exemplarFields...))),
}

const resourceExpr = "struct_update(resource, attributes := variant_bytes_to_variant(resource.attributes))::otel_resource"
const scopeExpr = "struct_update(scope, attributes := variant_bytes_to_variant(scope.attributes))::otel_scope"

func spanAppender(ctx context.Context, conn driver.Conn) *arrowingest.Appender {
	fields := arrowingest.TableFields("spans")
	fields = append(fields, field("events", arrow.ListOf(arrow.StructOf(eventFields...))), field("links", arrow.ListOf(arrow.StructOf(linkFields...))))
	query := `insert into spans select * replace(trace_id::uuid as trace_id,
 variant_bytes_to_variant(attributes) as attributes,` + resourceExpr + ` as resource,` + scopeExpr + ` as scope,
	list_transform(events,lambda e:struct_update(e,id:=e.id::uuid,attributes:=variant_bytes_to_variant(e.attributes))) as events,
	list_transform(links,lambda l:struct_update(l,id:=l.id::uuid,linked_trace_id:=l.linked_trace_id::uuid,attributes:=variant_bytes_to_variant(l.attributes))) as links)
 from appended_data`
	return arrowingest.NewCustomAppender(ctx, conn, fields, query)
}

func metricAppender(ctx context.Context, conn driver.Conn) *arrowingest.Appender {
	fields := arrowingest.TableFields("metrics")
	fields = append(fields, field("record_id", text), field("datapoints", arrow.ListOf(arrow.StructOf(pointFields...))), field("point_time_min", u64), field("point_time_max", u64))
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	query := `insert into metric_records(` + strings.Join(names, ",") + `) select * replace(
 id::uuid as id,resource_id::uuid as resource_id,resource_payload_id::uuid as resource_payload_id,scope_id::uuid as scope_id,
 variant_bytes_to_variant(metadata) as metadata,` + resourceExpr + ` as resource,` + scopeExpr + ` as scope,record_id::uuid as record_id,
	list_transform(datapoints,lambda d:struct_update(d,id:=d.id::uuid,attributes:=variant_bytes_to_variant(d.attributes),
	exemplars:=list_transform(d.exemplars,lambda e:struct_update(e,id:=e.id::uuid,trace_id:=e.trace_id::uuid,attributes:=variant_bytes_to_variant(e.attributes))))) as datapoints)
 from appended_data`
	return arrowingest.NewCustomAppender(ctx, conn, fields, query)
}
