create or replace macro otlp_resource(resource_id) as (
	select json_object(
		'attributes', otlp_attributes(r.attribute_ids),
		'droppedAttributesCount', r.dropped_attributes_count)
	from resources r
	where r.id = resource_id
)
