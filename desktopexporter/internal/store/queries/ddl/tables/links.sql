create table if not exists links (
		id uuid primary key,
		span_id uuid not null,
		trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		foreign key (span_id) references spans(span_id)
	)
