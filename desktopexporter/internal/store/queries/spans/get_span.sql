with target as materialized (
	select *
	from spans
	where trace_id = ?::uuid
		and span_id = (select unnest(?::ubigint[]))
), event_data as (
	select to_json(list(event_json(e, attrs_json(e.attribute_ids)) order by e.timestamp, e.id)) as events
	from events e
	where exists (select 1 from target t where t.trace_id = e.trace_id and t.span_id = e.span_id)
), link_data as (
	select to_json(list(link_json(l, attrs_json(l.attribute_ids)) order by l.id)) as links
	from links l
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
	'attributes', attrs_json(t.attribute_ids),
	'events', coalesce((select events from event_data), json('[]')),
	'links', coalesce((select links from link_data), json('[]')),
	'resource', resource_json(r.attribute_ids, r.dropped_attributes_count),
	'scope', scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count),
	'resourceSchemaURL', t.resource_schema_url,
	'scopeSchemaURL', t.scope_schema_url,
	'droppedAttributesCount', t.dropped_attributes_count,
	'droppedEventsCount', t.dropped_events_count,
	'droppedLinksCount', t.dropped_links_count,
	'statusCodeValue', t.status_code,
	'statusCode', case t.status_code when 0 then 'Unset' when 1 then 'Ok' when 2 then 'Error' else 'Unknown (' || t.status_code::varchar || ')' end,
	'statusMessage', t.status_message
) as varchar)
from target t
join resources r on r.id = t.resource_id
join scopes sc on sc.id = t.scope_id
