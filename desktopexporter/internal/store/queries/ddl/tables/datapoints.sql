create table if not exists datapoints (
		id uuid primary key,
		stream_id uuid not null,
		-- The series this point belongs to. Grouping a chart is now an
		-- equality on one fixed-width column instead of hashing a LIST, and
		-- it is indexable, which a LIST is not.
		--
		-- attribute_ids stays alongside it rather than being replaced: it is
		-- what the series id is derived from, it is what attrs_json renders,
		-- and dropping it would make the labels of a datapoint unreachable
		-- without a join back through metric_series.
		series_id uuid not null,
		metric_ingest_id uuid not null,
		timestamp bigint,
		start_time bigint,
		flags uinteger,
		double_value double,
		int_value bigint,
		value_type varchar,
		count ubigint,
		sum double,
		min double,
		max double,
		bucket_counts ubigint[],
		-- References histogram_bounds instead of carrying the vector. The
		-- reference costs a fixed 16 bytes; the vector it replaces repeats
		-- identically on every datapoint of a histogram series, and uuid
		-- columns do not dictionary-compress the way varchars do -- the
		-- dedupe has to be structural.
		bounds_id uuid,
		scale integer,
		zero_count ubigint,
		zero_threshold double,
		positive_bucket_offset integer,
		positive_bucket_counts ubigint[],
		negative_bucket_offset integer,
		negative_bucket_counts ubigint[],
		-- Replaces attrs_canonical. That column materialised the datapoint's
		-- attribute set as "key=value|..." so grouping by stream-within-stream
		-- was an equality compare on a varchar. The array serves the same
		-- purpose and is the identity itself rather than a rendering of it:
		-- equal arrays mean equal attribute sets, by construction.
		--
		-- This is where the rewrite pays most. On the reference capture,
		-- 294,607 datapoints carry 591,890 attribute rows resolving to 89
		-- distinct label sets -- 82% of the whole attributes table.
		attribute_ids uuid[] not null,
		foreign key (stream_id) references metric_streams(id),
		foreign key (series_id) references metric_series(id),
		foreign key (metric_ingest_id) references metric_ingests(id),
		foreign key (bounds_id) references histogram_bounds(id)
	)
