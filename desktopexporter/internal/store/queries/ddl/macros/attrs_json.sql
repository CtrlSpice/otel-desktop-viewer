-- attrs_json renders an owner's attribute_ids as the wire attribute array.
--
-- Replaces the per-owner attribute CTE that appeared eight times across
-- spans.go, logs.go and metrics.go. Ordered by key: array order is
-- identity, not presentation, so display order is imposed here.
create or replace macro attrs_json(ids) as (
		coalesce((
			select to_json(list(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)
			                    order by a.key, a.id))
			from unnest(ids) as t(aid)
			join attributes a on a.id = t.aid
		), json('[]'))
	)
