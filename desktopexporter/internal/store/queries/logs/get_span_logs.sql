select cast(coalesce(to_json(list(log_data_json(
	l,
	resource_json(r.attribute_ids, r.dropped_attributes_count),
	scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count)
) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp), l.id)), '[]') as varchar) as logs
from logs l
join resources r on r.id = l.resource_id
join scopes sc on sc.id = l.scope_id
where l.trace_id = ?::uuid
	and l.span_id = (select unnest(?::ubigint[]))
