create or replace macro otlp_attributes(ids) as (
	coalesce((
		select list(json_object('key', a.key, 'value', otlp_any_value(a.value)) order by a.key, a.id)
		from unnest(ids) as owner(attribute_id)
		join attributes a on a.id = owner.attribute_id
	), []::json[])
)
