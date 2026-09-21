-- Convert a batch of canonical stored pcommon.Values to OTLP AnyValues.
-- json_tree records each source's relationships once, recursion assigns numeric
-- sort paths to all nodes at each depth, and one ordered aggregation per source
-- joins local fragments. source_id keeps independent values from interleaving.
create or replace macro otlp_any_values(encoded_values) as table (
	with recursive
	otlp_av_input as materialized (
		select ordinality::bigint as source_id, encoded
		from unnest(case
			when encoded_values is null then error('stored OTel value batch is SQL NULL')
			else encoded_values::json[]
		end) with ordinality as sources(encoded, ordinality)
	),
	otlp_av_source_tree as materialized (
		select input.source_id, tree.id::bigint as id, tree.parent::bigint as parent,
			tree.key, tree.type, tree.atom
		from otlp_av_input input
		cross join lateral json_tree(case
			when input.encoded is null then error('stored OTel value is SQL NULL')
			else input.encoded
		end) tree
	),
	otlp_av_kinds(kind, otlp_field) as (
		values ('empty', null), ('string', 'stringValue'),
			('bool', 'boolValue'), ('int64', 'intValue'),
			('double', 'doubleValue'), ('bytes', 'bytesValue'),
			('array', 'arrayValue'), ('map', 'kvlistValue')
	),
	otlp_av_tagged as materialized (
		select tag.source_id, tag.parent as id,
			json_extract_string(tag.atom, '$') as kind,
			payload.id as payload_id, payload.type as payload_type, payload.atom as scalar
		from otlp_av_source_tree tag
		left join otlp_av_source_tree payload
			on payload.source_id = tag.source_id
			and payload.parent = tag.parent and payload.key = 'value'
		where tag.key = 'kind' and tag.type = 'VARCHAR'
	),
	otlp_av_edges as materialized (
		select child.source_id, child.id, parent.id as parent_id,
			child.key::bigint as ordinal, null::varchar as attribute_key
		from otlp_av_tagged parent
		join otlp_av_source_tree child
			on child.source_id = parent.source_id and child.parent = parent.payload_id
		where parent.kind = 'array'
		union all
		select child.source_id, child.id, parent.id as parent_id,
			entry.key::bigint as ordinal,
			json_extract_string(entry_key.atom, '$') as attribute_key
		from otlp_av_tagged parent
		join otlp_av_source_tree entry
			on entry.source_id = parent.source_id and entry.parent = parent.payload_id
		join otlp_av_source_tree child
			on child.source_id = entry.source_id
			and child.parent = entry.id and child.key = 'value'
		join otlp_av_source_tree entry_key
			on entry_key.source_id = entry.source_id
			and entry_key.parent = entry.id and entry_key.key = 'key'
		where parent.kind = 'map'
	),
	otlp_av_nodes as materialized (
		select n.*, e.parent_id, coalesce(e.ordinal, 0) as ordinal, e.attribute_key
		from otlp_av_tagged n
		left join otlp_av_edges e on e.source_id = n.source_id and e.id = n.id
	),
	otlp_av_child_bounds as (
		select source_id, parent_id, max(ordinal) + 2 as closing_position,
			count(*) as child_count
		from otlp_av_edges
		group by source_id, parent_id
	),
	otlp_av_fragments as materialized (
		select n.source_id, n.id, coalesce(c.closing_position, 1) as closing_position,
			case when n.ordinal > 0 then ',' else '' end
			|| case when p.kind = 'map'
				then '{"key":' || to_json(n.attribute_key)::varchar || ',"value":'
				else '' end
			|| case
				when n.kind = 'empty' then '{}'
				when n.kind in ('array', 'map') and n.payload_type != 'ARRAY'
					then error('stored OTel container value is not an array')
				when n.kind in ('array', 'map')
					then '{' || to_json(k.otlp_field)::varchar || ':{"values":['
				when k.otlp_field is null
					then error('unknown stored OTel value kind: ' || n.kind)
				when n.scalar is null then error('stored OTel scalar has no value')
				when n.kind in ('string', 'bytes', 'int64') and n.payload_type != 'VARCHAR'
					then error('stored OTel scalar has invalid value')
				when n.kind = 'bool' and n.payload_type != 'BOOLEAN'
					then error('stored OTel scalar has invalid value')
				when n.kind = 'int64' and (
					not regexp_full_match(json_extract_string(n.scalar, '$'), '-?(0|[1-9][0-9]*)')
					or try_cast(json_extract_string(n.scalar, '$') as bigint) is null)
					then error('stored OTel scalar has invalid value')
				when n.kind = 'bytes' and to_base64(from_base64(
					json_extract_string(n.scalar, '$'))) != json_extract_string(n.scalar, '$')
					then error('stored OTel scalar has invalid value')
				when n.kind = 'double' and n.payload_type not in ('BIGINT', 'UBIGINT', 'DOUBLE', 'VARCHAR')
					then error('stored OTel scalar has invalid value')
				when n.kind = 'double'
					then json_object(k.otlp_field, otlp_double_json(
						json_object('kind', 'double', 'value', n.scalar)))::varchar
				else json_object(k.otlp_field, n.scalar)::varchar
			end as opening,
			case when n.kind in ('array', 'map') then ']}}' else '' end
			|| case when p.kind = 'map' then '}' else '' end as closing
		from otlp_av_nodes n
		left join otlp_av_kinds k on k.kind = n.kind
		left join otlp_av_nodes p
			on p.source_id = n.source_id and p.id = n.parent_id
		left join otlp_av_child_bounds c
			on c.source_id = n.source_id and c.parent_id = n.id
	),
	otlp_av_node_paths(source_id, id, sort_path) as (
		select source_id, id, []::bigint[]
		from otlp_av_nodes
		where id = 0
		union all
		select child.source_id, child.id, parent.sort_path || [child.ordinal + 1]
		from otlp_av_node_paths parent
		join otlp_av_nodes child
			on child.source_id = parent.source_id and child.parent_id = parent.id
	),
	otlp_av_output_fragments as (
		select p.source_id, p.sort_path || [part.position] as sort_path, part.fragment
		from otlp_av_node_paths p
		join otlp_av_fragments f on f.source_id = p.source_id and f.id = p.id
		cross join lateral (
			values (0::bigint, f.opening), (f.closing_position, f.closing)
		) as part(position, fragment)
	),
	otlp_av_counts as (
		select input.source_id,
			(select count(*) from otlp_av_nodes n where n.source_id = input.source_id) as node_count,
			(select count(*) from otlp_av_edges e where e.source_id = input.source_id) as edge_count,
			(select count(*) from otlp_av_node_paths p where p.source_id = input.source_id) as path_count
		from otlp_av_input input
	)
	select counts.source_id, case
		when counts.path_count = 0 then error('stored OTel value has no tagged root')
		when counts.path_count != counts.node_count
			then error('stored OTel value contains a disconnected node')
		when counts.edge_count != counts.node_count - 1
			then error('stored OTel container contains a malformed child')
		when exists (
			select 1 from otlp_av_nodes n
			left join otlp_av_child_bounds children
				on children.source_id = n.source_id and children.parent_id = n.id
			where n.source_id = counts.source_id and n.kind in ('array', 'map') and (
				n.payload_type != 'ARRAY'
				or coalesce(children.child_count, 0) != json_array_length(n.scalar)
			)
		) then error('stored OTel container contains a malformed child')
		else string_agg(output.fragment, '' order by output.sort_path)::json
	end as value
	from otlp_av_counts counts
	left join otlp_av_output_fragments output using (source_id)
	group by counts.source_id, counts.node_count, counts.edge_count, counts.path_count
)
