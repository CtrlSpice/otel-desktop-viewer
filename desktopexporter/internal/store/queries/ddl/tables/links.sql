create table if not exists links (
		id uuid primary key,
		span_id uuid not null,
		trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		-- As on spans: W3C trace flags for the linked context.
		flags uinteger,
		foreign key (span_id) references spans(span_id)
	)
