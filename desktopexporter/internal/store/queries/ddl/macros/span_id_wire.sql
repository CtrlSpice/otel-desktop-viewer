create or replace macro span_id_wire(id) as (
		printf('%016x', id)
	)
