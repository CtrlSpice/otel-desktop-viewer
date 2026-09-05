-- exemplar_json renders one exemplar for the wire.
--
-- The third of the trio with event_json and link_json, and the one that was
-- left inline. Takes the exemplars row itself, so the shape lives beside the
-- other JSON shapers rather than inside whichever query needs it.
--
-- Unlike event_json this resolves its own attributes: an exemplar's filtered
-- attributes are few and the rows are few -- every capture in the reference
-- corpus contains zero exemplars -- so there is no per-row cost worth hoisting
-- out. If that changes, pass them in the way event_json does.
create or replace macro exemplar_json(e) as (
		json_object(
			'timestamp', e.timestamp::varchar,
			'valueType', case
				when e.double_value is not null then 'Double'
				when e.int_value is not null then 'Int'
				else 'Empty'
			end,
			'doubleValue', case
				when e.double_value is null then null::json
				when isfinite(e.double_value) then to_json(e.double_value)
				when isnan(e.double_value) then to_json('NaN')
				when e.double_value > 0 then to_json('Infinity')
				else to_json('-Infinity')
			end,
			'intValue', e.int_value::varchar,
			'traceID', trace_id_wire(e.trace_id),
			'spanID', span_id_wire(e.span_id),
			'filteredAttributes', attrs_json(e.attribute_ids)
		)
	)
