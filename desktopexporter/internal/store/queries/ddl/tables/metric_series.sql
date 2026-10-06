-- metric_series is the timeseries: the thing a chart draws one line per.
--
-- id is an opaque UUID generated when the exact parent Metric ID and datapoint
-- attributes first appear in this database. Resource and Scope identity is
-- already present in stream_id, so it is not duplicated here. The ID remains
-- stable for this persisted database and supports indexed grouping and URLs.
create table if not exists metric_series (
		id uuid primary key,
		stream_id uuid not null,
		attribute_ids uuid[] not null
	)
