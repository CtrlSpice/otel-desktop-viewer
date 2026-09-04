create table if not exists links (
		id uuid primary key,
		-- The owning span, by the pair that identifies one.
		trace_id uuid not null,
		span_id ubigint not null,
		-- The context pointed *at*, which may be in a different trace.
		-- Named to match linked_span_id beside it.
		linked_trace_id uuid,
		linked_span_id ubigint,
		trace_state varchar,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		-- As on spans: W3C trace flags for the linked context.
		flags uinteger,
		foreign key (trace_id, span_id) references spans(trace_id, span_id)
	)
