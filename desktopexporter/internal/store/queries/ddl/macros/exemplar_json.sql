-- exemplar_json renders one exemplar for the wire.
-- Filtered attributes are resolved per exemplar.
create or replace macro exemplar_json(e) as (
		json_object(
			'timestamp', e.timestamp::varchar,
			'valueType', case
				when e.double_value is not null then 'Double'
				when e.int_value is not null then 'Int'
				else 'Empty'
			end,
			'doubleValue', double_wire_json(e.double_value),
			'intValue', e.int_value::varchar,
			'traceID', trace_id_wire(e.trace_id),
			'spanID', span_id_wire(e.span_id),
			'filteredAttributes', attrs_json(e.attribute_ids)
		)
	)
