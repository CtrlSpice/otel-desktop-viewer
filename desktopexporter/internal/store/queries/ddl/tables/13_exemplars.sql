create table if not exists exemplars (
		id uuid primary key,
		datapoint_id uuid not null,
		timestamp bigint,
		value double,
		trace_id uuid,
		span_id uuid,
		attribute_ids uuid[] not null,
		foreign key (datapoint_id) references datapoints(id)
	)
