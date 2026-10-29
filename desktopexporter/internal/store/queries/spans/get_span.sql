{{if arrow}}with trace_spans as materialized (
	select * from spans where trace_id = try_cast($1 as uuid)
	and span_id = (select unnest($2::ubigint[]))
), tree as materialized (
	select *, 0 as depth, [1] as sort_path from trace_spans
), {{template "traceNative" .}}
{{else}}with target as materialized (
	select *
	from spans
	where trace_id = ?::uuid
		and span_id = (select unnest(?::ubigint[]))
), event_data as (
	select to_json(list(event_json(e, attribute_document(e.attributes)) order by e.timestamp, e.ordinal)) as events
	from events e
	where exists (select 1 from target t where t.trace_id = e.trace_id and t.span_id = e.span_id)
), link_data as (
	select to_json(list(link_json(l, attribute_document(l.attributes)) order by l.ordinal)) as links
	from span_link_summaries((select trace_id from target), (select span_id from target)) l
	where exists (select 1 from target t where t.trace_id = l.trace_id and t.span_id = l.span_id)
)
select cast(json_object(
	'traceID', trace_id_wire(t.trace_id),
	'traceState', t.trace_state,
	'spanID', span_id_wire(t.span_id),
	'parentSpanID', span_id_wire(t.parent_span_id),
	'flags', t.flags,
	'name', t.name,
	'kindCode', t.kind,
	'kind', case t.kind when 0 then 'Unspecified' when 1 then 'Internal' when 2 then 'Server' when 3 then 'Client' when 4 then 'Producer' when 5 then 'Consumer' else 'Unknown (' || t.kind::varchar || ')' end,
	'startTime', t.start_time::varchar,
	'endTime', t.end_time::varchar,
	'attributes', attribute_document(t.attributes),
	'events', coalesce((select events from event_data), json('[]')),
	'links', coalesce((select links from link_data), json('[]')),
	'resource', json_object('attributes', attribute_document(t.resource.attributes),
		'droppedAttributesCount', t.resource.dropped_attributes_count),
	'scope', json_object('name', t.scope.name, 'version', t.scope.version,
		'attributes', attribute_document(t.scope.attributes), 'droppedAttributesCount', t.scope.dropped_attributes_count),
	'resourceSchemaURL', t.resource.schema_url,
	'scopeSchemaURL', t.scope.schema_url,
	'droppedAttributesCount', t.dropped_attributes_count,
	'droppedEventsCount', t.dropped_events_count,
	'droppedLinksCount', t.dropped_links_count,
	'statusCodeValue', t.status_code,
	'statusCode', case t.status_code when 0 then 'Unset' when 1 then 'Ok' when 2 then 'Error' else 'Unknown (' || t.status_code::varchar || ')' end,
	'statusMessage', t.status_message
) as varchar)
from target t{{end}}
