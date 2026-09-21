create or replace macro otlp_scope(scope_id) as (
	select json_object(
		'name', s.name,
		'version', s.version,
		'attributes', otlp_attributes(s.attribute_ids),
		'droppedAttributesCount', s.dropped_attributes_count)
	from scopes s
	where s.id = scope_id
)
