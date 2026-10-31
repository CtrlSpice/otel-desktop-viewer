-- Values are encoded directly from the owning log's native payloads.
select otlp_document_text(json_object('resourceLogs', [json_object(
    'resource', otlp_resource(l.resource),
    'schemaUrl', l.resource.schema_url,
    'scopeLogs', [json_object(
        'scope', otlp_scope(l.scope),
        'schemaUrl', l.scope.schema_url,
        'logRecords', [json_merge_patch(json_object(
            'timeUnixNano', l.timestamp::varchar,
            'observedTimeUnixNano', l.observed_timestamp::varchar,
            'severityNumber', l.severity_number, 'severityText', l.severity_text,
            'body', otlp_value(l.body),
            'attributes', otlp_attributes(l.attributes),
            'droppedAttributesCount', l.dropped_attributes_count,
            'flags', l.flags, 'eventName', l.event_name),
            case when l.trace_id is null then '{}'::json else json_object('traceId', trace_id_wire(l.trace_id)) end,
            case when l.span_id is null then '{}'::json else json_object('spanId', span_id_wire(l.span_id)) end)]
    )]
)]))
from logs l where id = try_cast(? as uuid)
