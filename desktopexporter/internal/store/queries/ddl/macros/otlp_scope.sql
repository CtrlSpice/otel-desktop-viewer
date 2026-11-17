create or replace macro otlp_scope(s) as (
    json_object('name', s.name, 'version', s.version,
        'attributes', otlp_attributes(s.attributes),
        'droppedAttributesCount', s.dropped_attributes_count)
)
