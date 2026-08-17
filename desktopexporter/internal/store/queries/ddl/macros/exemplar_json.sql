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
			'value', e.value,
			'traceID', trace_id_wire(e.trace_id),
			'spanID', span_id_wire(e.span_id),
			'filteredAttributes', attrs_json(e.attribute_ids)
		)
	)
