create or replace macro otlp_attributes(ids, converted_values) as (
	coalesce((
		select list(json_object(
			'key', a.key,
			'value', converted.entry.value) order by a.key, a.id)
		from unnest(ids) as owner(attribute_id)
		join attributes a on a.id = owner.attribute_id
		join unnest(converted_values) as converted(entry) on converted.entry.id = a.id
	), []::json[])
)
