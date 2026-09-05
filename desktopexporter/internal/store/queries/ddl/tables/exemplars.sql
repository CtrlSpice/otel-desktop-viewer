create table if not exists exemplars (
		id uuid primary key,
		datapoint_id uuid not null,
		timestamp bigint,
		double_value double,
		int_value bigint,
		trace_id uuid,
		span_id ubigint,
		attribute_ids uuid[] not null,
		check (double_value is null or int_value is null),
		foreign key (datapoint_id) references datapoints(id)
	)
