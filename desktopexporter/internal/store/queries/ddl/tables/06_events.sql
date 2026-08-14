create table if not exists events (
		id uuid primary key,
		span_id uuid not null,
		name varchar,
		timestamp bigint,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		foreign key (span_id) references spans(span_id)
	)
