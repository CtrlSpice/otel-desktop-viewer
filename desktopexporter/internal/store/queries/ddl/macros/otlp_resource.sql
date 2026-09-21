create or replace macro otlp_resource(resource_id, converted_values) as (
	select json_object(
		'attributes', otlp_attributes(r.attribute_ids, converted_values),
		'droppedAttributesCount', r.dropped_attributes_count)
	from resources r
	where r.id = resource_id
)
