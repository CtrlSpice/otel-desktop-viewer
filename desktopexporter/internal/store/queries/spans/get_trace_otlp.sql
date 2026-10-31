-- Export every stored span in the requested trace, including unreachable spans.
with selected as materialized (
    select * from spans s where s.trace_id = try_cast(? as uuid)
        and (? = '' or s.span_id = try_cast(? as ubigint))
), event_documents as (
    select e.trace_id, e.span_id, list(json_object(
        'timeUnixNano', e.timestamp::varchar, 'name', e.name,
        'attributes', otlp_attributes(e.attributes),
        'droppedAttributesCount', e.dropped_attributes_count) order by e.timestamp, e.ordinal) as documents
    from events e join selected using(trace_id, span_id)
    group by e.trace_id, e.span_id
), link_documents as (
    select l.trace_id, l.span_id, list(json_merge_patch(json_object(
        'traceState', l.trace_state,
        'attributes', otlp_attributes(l.attributes),
        'droppedAttributesCount', l.dropped_attributes_count, 'flags', l.flags),
        case when l.linked_trace_id is null then '{}'::json else json_object('traceId', trace_id_wire(l.linked_trace_id)) end,
        case when l.linked_span_id is null then '{}'::json else json_object('spanId', span_id_wire(l.linked_span_id)) end
    ) order by l.ordinal) as documents
    from links l join selected using(trace_id, span_id)
    group by l.trace_id, l.span_id
), span_documents as (
    select s.resource.id as resource_id, s.resource.schema_url as resource_schema_url,
        s.scope.id as scope_id, s.scope.schema_url as scope_schema_url,
        otlp_resource(s.resource) as resource,
        otlp_scope(s.scope) as scope,
        s.start_time, s.span_id,
        json_merge_patch(json_object(
            'traceId', trace_id_wire(s.trace_id), 'spanId', span_id_wire(s.span_id),
            'traceState', s.trace_state, 'name', s.name, 'kind', s.kind,
            'startTimeUnixNano', s.start_time::varchar, 'endTimeUnixNano', s.end_time::varchar,
            'attributes', otlp_attributes(s.attributes),
            'droppedAttributesCount', s.dropped_attributes_count,
            'events', coalesce(e.documents, []::json[]),
            'droppedEventsCount', s.dropped_events_count,
            'links', coalesce(l.documents, []::json[]),
            'droppedLinksCount', s.dropped_links_count,
            'status', json_object('message', s.status_message, 'code', s.status_code), 'flags', s.flags),
            case when s.parent_span_id is null then '{}'::json else json_object('parentSpanId', span_id_wire(s.parent_span_id)) end
        ) as document
    from selected s
    left join event_documents e on e.trace_id=s.trace_id and e.span_id=s.span_id
    left join link_documents l on l.trace_id=s.trace_id and l.span_id=s.span_id
), scope_groups as (
    select resource_id, resource_schema_url, resource, scope_id, scope_schema_url,
        json_object('scope', scope, 'spans', list(document order by start_time, span_id),
            'schemaUrl', scope_schema_url) as document
    from span_documents group by resource_id, resource_schema_url, resource, scope_id, scope_schema_url, scope
), resource_groups as (
    select resource_id, resource_schema_url,
        json_object('resource', resource, 'scopeSpans', list(document order by scope_schema_url, scope_id),
            'schemaUrl', resource_schema_url) as document
    from scope_groups group by resource_id, resource_schema_url, resource
)
select otlp_document_text(json_object('resourceSpans', list(document order by resource_schema_url, resource_id)))
from resource_groups having count(*) > 0
