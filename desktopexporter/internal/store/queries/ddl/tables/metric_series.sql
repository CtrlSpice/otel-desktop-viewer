-- metric_series is the timeseries: the thing a chart draws one line per.
--
-- id is a versioned content ID over the exact parent Metric ID and datapoint
-- attributes. Resource and Scope identity is already present in stream_id, so
-- it is not duplicated here. The stable ID supports indexed grouping and URLs.
create table if not exists metric_series (
		id uuid primary key,
		stream_id uuid not null,
		attribute_ids uuid[] not null,
		foreign key (stream_id) references metric_streams(id)
	)
