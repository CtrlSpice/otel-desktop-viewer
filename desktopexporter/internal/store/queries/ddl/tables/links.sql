create table if not exists links (
		id uuid primary key,
		-- The owning span, by the pair that identifies one.
		trace_id uuid not null,
		span_id uuid not null,
		-- The context pointed *at*, which is a different trace entirely.
		-- Named to match linked_span_id beside it.
		linked_trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		-- As on spans: W3C trace flags for the linked context.
		flags uinteger,
		foreign key (trace_id, span_id) references spans(trace_id, span_id)
	)
