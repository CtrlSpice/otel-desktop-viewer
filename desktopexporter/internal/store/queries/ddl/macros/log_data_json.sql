-- log_data_json is the full UI-shaped projection for one stored log row.
-- Single-log and span-correlated reads share it so their fields cannot drift.
create or replace macro log_data_json(l, attributes, resource, scope, resource_schema_url, scope_schema_url) as (
    json_object(
        'logRef', l.id,
        'timestamp', l.timestamp::varchar,
        'observedTimestamp', l.observed_timestamp::varchar,
        'traceID', trace_id_wire(l.trace_id),
        'spanID', span_id_wire(l.span_id),
        'severityText', l.severity_text,
        'severityNumber', l.severity_number,
        'body', otel_value_json(l.body),
        'resource', resource,
        'scope', scope,
        'resourceSchemaURL', resource_schema_url,
        'scopeSchemaURL', scope_schema_url,
        'droppedAttributesCount', l.dropped_attributes_count,
        'flags', l.flags,
        'eventName', l.event_name,
        'attributes', attributes
    )
)
