create or replace macro otlp_resource(r) as (
    json_object('attributes', otlp_attributes(r.attributes),
        'droppedAttributesCount', r.dropped_attributes_count)
)
