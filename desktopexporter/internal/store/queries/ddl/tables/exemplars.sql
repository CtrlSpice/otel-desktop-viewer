create table if not exists exemplars (
		id uuid primary key,
		metric_datapoint_id uuid not null,
		timestamp ubigint,
		double_value double,
		int_value bigint,
		trace_id uuid,
		span_id ubigint,
		attributes variant not null,
		check (double_value is null or int_value is null),
		foreign key (metric_datapoint_id) references metric_datapoints(id)
	)
