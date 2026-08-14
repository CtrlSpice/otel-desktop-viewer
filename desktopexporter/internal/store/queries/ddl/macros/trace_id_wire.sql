-- Wire renderings of the two id types. Trace ids go out as 32 hex chars and
-- span ids as the low 16, matching the OTLP/JSON convention.
create or replace macro trace_id_wire(id) as (
		replace(id::varchar, '-', '')
	)
