
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope,
			'type', sub.type) order by sub.key, sub.scope, sub.type)) as varchar) as attributes
		from (
			select distinct a.key, 'resource' as scope, json_extract_string(a.value, '$.kind') as type from metric_ingests m join resources r on r.id = m.resource_id, unnest(r.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'scope', json_extract_string(a.value, '$.kind') from metric_ingests m join scopes s on s.id = m.scope_id, unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'datapoint', json_extract_string(a.value, '$.kind') from datapoints d, unnest(d.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'exemplar', json_extract_string(a.value, '$.kind') from exemplars e, unnest(e.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'metadata', json_extract_string(a.value, '$.kind') from metric_ingests m, unnest(m.metadata_ids) t(aid) join attributes a on a.id = t.aid
		) sub
