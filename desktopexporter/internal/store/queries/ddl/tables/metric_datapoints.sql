create table if not exists metric_datapoints (
		id uuid primary key,
		metric_id uuid not null,
		-- Fixed-width series reference for indexed grouping.
		series_id uuid not null,
		timestamp ubigint,
		start_time ubigint,
		flags uinteger,
		double_value double,
		int_value bigint,
		value_type varchar,
		count ubigint,
		-- Optional OTLP histogram statistics. NULL means absent; zero means
		-- present with value zero for both histogram representations.
		sum double,
		min double,
		max double,
		bucket_counts ubigint[],
		-- Shared explicit-bounds vector.
		bounds_id uuid,
		scale integer,
		zero_count ubigint,
		zero_threshold double,
		positive_bucket_offset integer,
		positive_bucket_counts ubigint[],
		negative_bucket_offset integer,
		negative_bucket_counts ubigint[],
		-- Sorted attribute IDs are the exact datapoint attribute-set identity.
		attribute_ids uuid[] not null,
		foreign key (series_id) references metric_series(id),
		foreign key (bounds_id) references histogram_bounds(id)
	)
