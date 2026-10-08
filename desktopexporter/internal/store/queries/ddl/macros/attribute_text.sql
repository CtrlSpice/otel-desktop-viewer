-- Text projection of a canonical stored value, following pcommon.Value.AsString:
-- scalars use their textual value; arrays/maps use JSON with untagged children.
-- This is a derived label, never a replacement for the stored typed value.
create or replace macro attribute_text(encoded) as (
	with recursive nodes(value, path, member_key) as (
		select encoded::json, []::bigint[], null::varchar
		union all
		select case when json_extract_string(n.value, '$.kind') = 'map'
			then child.value->'value' else child.value end,
			n.path || [child.key::bigint + 1],
			case when json_extract_string(n.value, '$.kind') = 'map'
				then json_extract_string(child.value, '$.key') else null end
		from nodes n, lateral json_each(n.value->'value') child
		where json_extract_string(n.value, '$.kind') in ('array', 'map')
	), scalars as (
		select *, json_extract_string(value, '$.kind') as kind,
			case
				when json_extract_string(value, '$.kind') = 'empty' then ''
				when json_extract_string(value, '$.kind') = 'double' then
					case
						when json_extract_string(value, '$.value') = '0x8000000000000000' then '-0'
						when isnan(attribute_double(value)) then 'NaN'
						when attribute_double(value) = 'Infinity'::double then 'Infinity'
						when attribute_double(value) = '-Infinity'::double then '-Infinity'
						-- json_extract_string removes the positive exponent sign from
						-- the stored Go JSON number. Restore it to match AsString.
						else regexp_replace(json_extract_string(value, '$.value'), 'e([0-9]+)$', 'e+\1')
					end
				else json_extract_string(value, '$.value')
			end as text
		from nodes
	), fragments as (
		select path || [0] as position,
			case when len(path) > 0 and path[-1] > 1 then ',' else '' end
			|| case when member_key is not null then
				replace(replace(to_json(member_key)::varchar, chr(8232), '\u2028'), chr(8233), '\u2029') || ':'
				else '' end
			|| case
				when kind = 'array' then '['
				when kind = 'map' then '{'
				when len(path) = 0 then text
				when kind = 'empty' then 'null'
				when kind in ('string', 'bytes') then
					replace(replace(to_json(text)::varchar, chr(8232), '\u2028'), chr(8233), '\u2029')
				else text
			end as fragment
		from scalars
		union all
		select path || [json_array_length(value->'value')::bigint + 1],
			case when kind = 'array' then ']' else '}' end
		from scalars where kind in ('array', 'map')
	)
	select case
		-- AsString marshals a container's raw values using encoding/json; a
		-- nested non-finite float makes that conversion return an empty string.
		when exists (select 1 from scalars where len(path) > 0 and kind = 'double'
			and not isfinite(attribute_double(value))) then ''
		else string_agg(fragment, '' order by position)
	end from fragments
)
