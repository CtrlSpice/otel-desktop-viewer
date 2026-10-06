-- event_json renders one span event for the wire.
-- attrs is pre-resolved to avoid a correlated lookup per event.
create or replace macro event_json(e, attrs) as (
    json_object(
        'name', e.name,
        'timestamp', e.timestamp::varchar,
        'droppedAttributesCount', e.dropped_attributes_count,
        'attributes', coalesce(attrs, json('[]'))
    )
)
