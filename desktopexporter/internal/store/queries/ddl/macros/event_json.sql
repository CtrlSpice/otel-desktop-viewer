-- event_json renders one span event for the wire.
--
-- Takes the events row itself, so the shape lives next to the other JSON
-- shapers rather than inline in whichever query happens to need it. attrs is
-- passed in rather than resolved here: the caller has already resolved every
-- event's attributes in one pass, and re-resolving per row is the mistake
-- attrs_mapped exists to avoid.
create or replace macro event_json(e, attrs) as (
    json_object(
        'name', e.name,
        'timestamp', e.timestamp::varchar,
        'droppedAttributesCount', e.dropped_attributes_count,
        'attributes', coalesce(attrs, json('[]'))
    )
)
