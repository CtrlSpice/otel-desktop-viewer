-- Native owner attributes projected to the existing UI value contract.
create or replace macro attribute_document(attrs) as (
    coalesce((
        select to_json(list(json_object(
            'id', attr_id(entry.value->>'key', (entry.value->'value')::varchar)::varchar,
            'key', entry.value->>'key', 'value', entry.value->'value') order by entry.value->>'key'))
        from json_each(otel_value_json(attrs), '$.value') entry
    ), json('[]'))
)
