-- Reconstruct every stored span carrying the bound trace ID. Membership is not
-- derived from parent reachability, search state, a time range, or UI collapse.
with selected as (
	select s.*
	from spans s
	where s.trace_id = try_cast(? as uuid)
),
span_documents as (
	select
		s.resource_id,
		s.resource_schema_url,
		s.scope_id,
		s.scope_schema_url,
		s.start_time,
		s.span_id,
		json_merge_patch(
			json_object(
				'traceId', trace_id_wire(s.trace_id),
				'spanId', span_id_wire(s.span_id),
				'traceState', s.trace_state,
				'name', s.name,
				'kind', s.kind,
				'startTimeUnixNano', s.start_time::varchar,
				'endTimeUnixNano', s.end_time::varchar,
				'attributes', otlp_attributes(s.attribute_ids),
				'droppedAttributesCount', s.dropped_attributes_count,
				'events', coalesce((
					select list(json_object(
						'timeUnixNano', e.timestamp::varchar,
						'name', e.name,
						'attributes', otlp_attributes(e.attribute_ids),
						'droppedAttributesCount', e.dropped_attributes_count)
						order by e.timestamp, e.id)
					from events e
					where e.trace_id = s.trace_id and e.span_id = s.span_id
				), []::json[]),
				'droppedEventsCount', s.dropped_events_count,
				'links', coalesce((
					select list(json_merge_patch(
						json_object(
							'traceState', l.trace_state,
							'attributes', otlp_attributes(l.attribute_ids),
							'droppedAttributesCount', l.dropped_attributes_count,
							'flags', l.flags),
						case when l.linked_trace_id is null then json('{}') else json_object('traceId', trace_id_wire(l.linked_trace_id)) end,
						case when l.linked_span_id is null then json('{}') else json_object('spanId', span_id_wire(l.linked_span_id)) end)
						order by l.id)
					from links l
					where l.trace_id = s.trace_id and l.span_id = s.span_id
				), []::json[]),
				'droppedLinksCount', s.dropped_links_count,
				'status', json_object('message', s.status_message, 'code', s.status_code),
				'flags', s.flags),
			case when s.parent_span_id is null then json('{}') else json_object('parentSpanId', span_id_wire(s.parent_span_id)) end
		) as document
	from selected s
),
scope_groups as (
	select resource_id, resource_schema_url, scope_id, scope_schema_url,
		json_object(
			'scope', otlp_scope(scope_id),
			'spans', list(document order by start_time, span_id),
			'schemaUrl', scope_schema_url) as document
	from span_documents
	group by resource_id, resource_schema_url, scope_id, scope_schema_url
),
resource_groups as (
	select resource_id, resource_schema_url,
		json_object(
			'resource', otlp_resource(resource_id),
			'scopeSpans', list(document order by scope_schema_url, scope_id),
			'schemaUrl', resource_schema_url) as document
	from scope_groups
	group by resource_id, resource_schema_url
)
select otlp_document_text(json_object(
	'resourceSpans', list(document order by resource_schema_url, resource_id)))
from resource_groups
having count(*) > 0
