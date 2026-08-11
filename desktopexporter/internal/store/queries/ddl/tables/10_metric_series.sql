-- metric_series is the timeseries: the thing a chart draws one line per.
--
-- The schema had no row for it. metric_streams is deliberately coarser --
-- it identifies a stream by service_name rather than by resource, so a
-- counter stays one timeseries when a pod restarts -- and the actual series
-- was only ever implied, by grouping datapoints on their label set at query
-- time. That had three costs:
--
-- - Two replicas of one service, emitting the same instrument with the
-- same labels, grouped together and interleaved into one line.
-- - Grouping ran on a LIST column, which DuckDB cannot index and must
-- rebuild and hash per row. Measured on 294,607 datapoints: 5.0ms
-- grouping by the array against 0.9ms by a fixed-width key.
-- - A series had no name, so nothing could link to one. Metric URLs could
-- only reference a datapoint id, which retention deletes -- shared links
-- rotted, and degraded silently to "no selection".
--
-- id = sha256(stream_id, resource_id, attribute_ids), so it is stable
-- across restarts and re-ingests: the same series always has the same id,
-- which is what makes it safe to put in a URL.
create table if not exists metric_series (
		id uuid primary key,
		stream_id uuid not null,
		resource_id uuid not null,
		attribute_ids uuid[] not null,
		foreign key (stream_id) references metric_streams(id),
		foreign key (resource_id) references resources(id)
	)
