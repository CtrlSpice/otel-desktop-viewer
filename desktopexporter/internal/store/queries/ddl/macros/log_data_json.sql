-- log_data_json is the one full UI-shaped projection for stored log rows.
-- Single-log and bulk trace-detail reads call the same shaper so fields cannot
-- drift. The caller owns joins, filtering, and ordering.
create or replace macro log_data_json(l, resource, scope) as (
    json_object(
        'id', l.id,
        'timestamp', l.timestamp::varchar,
        'observedTimestamp', l.observed_timestamp::varchar,
        'traceID', trace_id_wire(l.trace_id),
        'spanID', span_id_wire(l.span_id),
        'severityText', l.severity_text,
        'severityNumber', l.severity_number,
        'body', l.body,
        'resource', resource,
        'scope', scope,
        'resourceSchemaURL', l.resource_schema_url,
        'scopeSchemaURL', l.scope_schema_url,
        'droppedAttributesCount', l.dropped_attributes_count,
        'flags', l.flags,
        'eventName', l.event_name,
        'attributes', attrs_json(l.attribute_ids)
    )
)
