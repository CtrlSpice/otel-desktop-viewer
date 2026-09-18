
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope,
			'type', sub.type) order by sub.key, sub.scope, sub.type)) as varchar) as attributes
		from (
			select distinct a.key, 'resource' as scope, json_extract_string(a.value, '$.kind') as type
			from logs l join resources r on r.id = l.resource_id, unnest(r.attribute_ids) t(aid)
			join attributes a on a.id = t.aid
			union
			select distinct a.key, 'scope', json_extract_string(a.value, '$.kind')
			from logs l join scopes s on s.id = l.scope_id, unnest(s.attribute_ids) t(aid)
			join attributes a on a.id = t.aid
			union
			select distinct a.key, 'log', json_extract_string(a.value, '$.kind')
			from logs l, unnest(l.attribute_ids) t(aid) join attributes a on a.id = t.aid
		) sub
