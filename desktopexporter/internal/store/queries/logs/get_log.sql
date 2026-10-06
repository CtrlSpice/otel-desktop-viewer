select cast(log_data_json(
	l,
	resource_json(r.attribute_ids, r.dropped_attributes_count),
	scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count),
	r.schema_url,
	sc.schema_url
) as varchar) as log
from logs l
join resources r on r.id = l.resource_id
join scopes sc on sc.id = l.scope_id
where l.id = ?::uuid
