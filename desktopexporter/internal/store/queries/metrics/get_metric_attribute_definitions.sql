
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope,
			'type', sub.type) order by sub.key, sub.scope, sub.type)) as varchar) as attributes
		from (
			select distinct a.key, 'resource' as scope, otel_value_kind(a.value) as type from metrics m join resources r on r.id = m.resource_id, unnest(r.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'scope', otel_value_kind(a.value) from metrics m join scopes s on s.id = m.scope_id, unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'datapoint', otel_value_kind(a.value) from metric_datapoints d, unnest(d.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'exemplar', otel_value_kind(a.value) from exemplars e, unnest(e.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'metadata', otel_value_kind(a.value) from metrics m, unnest(m.metadata_ids) t(aid) join attributes a on a.id = t.aid
		) sub
