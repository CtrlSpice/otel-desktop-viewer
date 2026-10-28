create table if not exists events (
		id uuid primary key,
		-- The owning span's trace. Part of the key that reaches it: a span id
		-- alone does not identify a span across traces.
		trace_id uuid not null,
		span_id ubigint not null,
		name varchar,
		timestamp ubigint,
		attributes variant not null,
		dropped_attributes_count uinteger,
		ordinal uinteger not null default 0,
		foreign key (trace_id, span_id) references spans(trace_id, span_id)
	)
