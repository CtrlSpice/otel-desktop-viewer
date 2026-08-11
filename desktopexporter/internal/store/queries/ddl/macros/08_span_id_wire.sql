create or replace macro span_id_wire(id) as (
		right(replace(id::varchar, '-', ''), 16)
	)
