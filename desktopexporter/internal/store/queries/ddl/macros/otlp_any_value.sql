-- Recursively convert one canonical stored pcommon.Value to an OTLP AnyValue.
-- Nodes at the same depth are converted together, deepest-first. The fold uses
-- fixed-width node IDs and retains only the direct-child layer needed next.
create or replace macro otlp_any_value(encoded_value) as (
	with recursive value_nodes(node_key, parent_key, child_ordinal, node_depth, encoded) as (
		select 'r'::varchar, null::varchar, 0::bigint, 0::bigint, case
			when encoded_value is null then error('stored OTel value is SQL NULL')
			else encoded_value::json
		end
		union all
		select n.node_key || '.' || child.ordinality::varchar, n.node_key,
			child.ordinality, n.node_depth + 1, child.encoded
		from value_nodes n
		cross join unnest(
			case json_extract_string(n.encoded, '$.kind')
				when 'array' then json_extract(n.encoded, '$.value[*]')
				when 'map' then list_transform(json_extract(n.encoded, '$.value[*]'), entry -> json_extract(entry, '$.value'))
				else []::json[]
			end
		) with ordinality as child(encoded, ordinality)
	),
	numbered as (
		select row_number() over (order by node_key)::bigint as node_id, *
		from value_nodes
	),
	with_children as (
		select node.node_id, node.node_key, node.node_depth, node.encoded,
			coalesce(list(child.node_id order by child.child_ordinal)
				filter (where child.node_id is not null), []::bigint[]) as child_ids
		from numbered node
		left join numbered child on child.parent_key = node.node_key
		group by node.node_id, node.node_key, node.node_depth, node.encoded
	),
	layers as (
		select node_depth,
			list(struct_pack(
				node_id := node_id,
				encoded := encoded,
				child_ids := child_ids)
				order by node_id) as nodes
		from with_children
		group by node_depth
	),
	fold(node_depth, values_by_node) as (
		select max(node_depth) + 1, map([]::bigint[], []::json[])
		from with_children
		union all
		select fold.node_depth - 1,
			map(
				list_transform(layer.nodes, node -> node.node_id),
				list_transform(layer.nodes, node ->
					case json_extract_string(node.encoded, '$.kind')
						when 'empty' then json('{}')
						when 'string' then json_object('stringValue', json_extract_string(node.encoded, '$.value'))
						when 'bool' then json_object('boolValue', json_extract(node.encoded, '$.value'))
						when 'int64' then json_object('intValue', json_extract_string(node.encoded, '$.value'))
						when 'double' then json_object('doubleValue', otlp_double_json(node.encoded))
						when 'bytes' then json_object('bytesValue', json_extract_string(node.encoded, '$.value'))
						when 'array' then json_object('arrayValue', json_object('values', list_transform(
							node.child_ids,
							child_id -> map_extract_value(fold.values_by_node, child_id))))
						when 'map' then json_object('kvlistValue', json_object('values', list_transform(
							node.child_ids,
							(child_id, child_index) -> json_object(
								'key', json_extract_string(node.encoded, '$.value[' || (child_index - 1)::varchar || '].key'),
								'value', map_extract_value(fold.values_by_node, child_id)))))
						else error('unknown stored OTel value kind: ' || coalesce(json_extract_string(node.encoded, '$.kind'), '<null>'))
					end
			))
		from fold
		join layers layer on layer.node_depth = fold.node_depth - 1
	)
	select map_extract_value(values_by_node,
		(select node_id from numbered where parent_key is null))
	from fold
	order by node_depth
	limit 1
)
