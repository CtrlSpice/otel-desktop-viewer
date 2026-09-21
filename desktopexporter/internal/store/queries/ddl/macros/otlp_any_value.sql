-- Recursively convert one canonical stored pcommon.Value to an OTLP AnyValue.
-- Nodes at the same depth are converted together, deepest-first, so runtime is
-- bounded by tree depth rather than by a quadratic node-at-a-time fold.
create or replace macro otlp_any_value(encoded_value) as (
	with recursive value_nodes(node_key, node_depth, encoded) as (
		select 'r'::varchar, 0::bigint, case
			when encoded_value is null then error('stored OTel value is SQL NULL')
			else encoded_value::json
		end
		union all
		select n.node_key || '.' || child.ordinality::varchar, n.node_depth + 1, child.encoded
		from value_nodes n
		cross join unnest(
			case json_extract_string(n.encoded, '$.kind')
				when 'array' then json_extract(n.encoded, '$.value[*]')
				when 'map' then list_transform(json_extract(n.encoded, '$.value[*]'), entry -> json_extract(entry, '$.value'))
				else []::json[]
			end
		) with ordinality as child(encoded, ordinality)
	),
	layers as (
		select node_depth,
			list(struct_pack(node_key := node_key, encoded := encoded) order by node_key) as nodes
		from value_nodes
		group by node_depth
	),
	fold(node_depth, values_by_node) as (
		select max(node_depth) + 1, map([]::varchar[], []::json[])
		from value_nodes
		union all
		select fold.node_depth - 1,
			map_concat(fold.values_by_node, map(
				list_transform(layer.nodes, node -> node.node_key),
				list_transform(layer.nodes, node ->
					case json_extract_string(node.encoded, '$.kind')
						when 'empty' then json('{}')
						when 'string' then json_object('stringValue', json_extract_string(node.encoded, '$.value'))
						when 'bool' then json_object('boolValue', json_extract(node.encoded, '$.value'))
						when 'int64' then json_object('intValue', json_extract_string(node.encoded, '$.value'))
						when 'double' then json_object('doubleValue', otlp_double_json(node.encoded))
						when 'bytes' then json_object('bytesValue', json_extract_string(node.encoded, '$.value'))
						when 'array' then json_object('arrayValue', json_object('values', list_transform(
							range(1, json_array_length(json_extract(node.encoded, '$.value'))::bigint + 1),
							i -> map_extract_value(fold.values_by_node, node.node_key || '.' || i::varchar))))
						when 'map' then json_object('kvlistValue', json_object('values', list_transform(
							range(1, json_array_length(json_extract(node.encoded, '$.value'))::bigint + 1),
							i -> json_object(
								'key', json_extract_string(node.encoded, '$.value[' || (i - 1)::varchar || '].key'),
								'value', map_extract_value(fold.values_by_node, node.node_key || '.' || i::varchar)))))
						else error('unknown stored OTel value kind: ' || coalesce(json_extract_string(node.encoded, '$.kind'), '<null>'))
					end
			)))
		from fold
		join layers layer on layer.node_depth = fold.node_depth - 1
	)
	select map_extract_value(values_by_node, 'r')
	from fold
	order by node_depth
	limit 1
)
