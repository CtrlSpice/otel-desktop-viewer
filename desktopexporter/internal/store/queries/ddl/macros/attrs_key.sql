-- attrs_key renders an attribute set as the canonical "key=value|..."
-- string, keys in lexicographic order.
-- It is a wire projection, not series identity.
create or replace macro attrs_key(ids) as (
		coalesce((
		select string_agg(a.key || '=' || coalesce(json_extract_string(a.value, '$.value'), a.value::varchar), '|' order by a.key, a.id)
			from unnest(ids) as t(aid)
			join attributes a on a.id = t.aid
		), '')
	)
