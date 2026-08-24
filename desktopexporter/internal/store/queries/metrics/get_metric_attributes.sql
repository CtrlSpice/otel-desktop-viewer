
		select cast(to_json(list(attribute_def_json(sub.key, sub.scope, sub.type)
			order by sub.key, sub.scope, sub.type::varchar)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			where a.scope in ('resource', 'scope', 'datapoint', 'exemplar', 'metadata')
		) sub
	