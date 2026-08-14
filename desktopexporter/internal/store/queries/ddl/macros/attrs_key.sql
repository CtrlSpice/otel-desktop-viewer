-- attrs_key renders an attribute set as the canonical "key=value|..."
-- string, keys in lexicographic order.
--
-- This is the old datapoints.attrs_canonical column, computed on demand
-- from attribute_ids instead of materialised at ingest. It survives only
-- because JsonMetricTimeseries.attributesKey is still that string on the
-- wire; when series identity gains the resource, this becomes a composite
-- and the macro goes with it.
--
-- Not an identity primitive any more: grouping is by attribute_ids, which
-- is the identity itself. This only renders it.
create or replace macro attrs_key(ids) as (
		coalesce((
			select string_agg(a.key || '=' || a.value, '|' order by a.key, a.id)
			from unnest(ids) as t(aid)
			join attributes a on a.id = t.aid
		), '')
	)
