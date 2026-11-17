package searchindex

import (
	"context"
	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

type serviceResource struct {
	resource, service string
	attributes        []byte
}

// Service identities reuse the catalogue's typed value identities. Resource
// schema, scope, and unrelated attributes do not participate in the pair.
func (b *Batch) writeServices(ctx context.Context, conn driver.Conn) error {
	if len(b.services) == 0 {
		return nil
	}
	defer func() { b.services = nil }()
	rows := array.NewRecordBuilder(arrowbridge.Allocator(), arrow.NewSchema([]arrow.Field{
		{Name: "resource_id", Type: arrow.BinaryTypes.String},
		{Name: "service_ref", Type: arrow.BinaryTypes.String},
		{Name: "attributes", Type: arrow.BinaryTypes.Binary},
	}, nil))
	defer rows.Release()
	for _, r := range b.services {
		rows.Field(0).(*array.StringBuilder).Append(r.resource)
		rows.Field(1).(*array.StringBuilder).Append(r.service)
		rows.Field(2).(*array.BinaryBuilder).Append(r.attributes)
	}
	release, err := streamRegister(ctx, conn, "stream_services", rows)
	if err != nil {
		return err
	}
	defer release()
	exec := connection{conn}
	// Format labels only for new services. Filtering and ordering use the exact
	// labels; Arrow summaries use bounded previews. Native telemetry stays owned
	// by its received record, including the values used for exact inspection.
	if _, err = exec.ExecContext(ctx, `insert into search_services
 with fresh as materialized (
   select distinct on(service_ref) service_ref, attributes
   from stream_services i where not exists(select 1 from search_services s where s.ref=i.service_ref)
   order by service_ref, resource_id
 ), fields as materialized (
   select service_ref, variant_exists(a,'service.name') as present,
     a['service.name'] as name,
     case when variant_exists(a,'service.namespace') then a['service.namespace'] else ''::variant end as namespace
   from (select service_ref, variant_bytes_to_variant(attributes) a from fresh)
 )
 select service_ref, coalesce(otel_value_text(name),''), coalesce(otel_value_text(namespace),''),
   case when present then otel_value_kind(name) else 'missing' end, otel_value_kind(namespace),
   case when not present then '' when otel_value_kind(name)='string' then
       case when length(otel_value_text(name))>300 then left(otel_value_text(name),299)||'…' else otel_value_text(name) end
       else native_column_preview(name) end,
   case when otel_value_kind(namespace)='string' then
       case when length(otel_value_text(namespace))>300 then left(otel_value_text(namespace),299)||'…' else otel_value_text(namespace) end
       else native_column_preview(namespace) end
 from fields`); err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `insert into search_service_resources
 select resource_id::uuid,service_ref from stream_services on conflict(resource_id) do nothing`)
	return err
}
