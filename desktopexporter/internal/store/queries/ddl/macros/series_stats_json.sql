-- series_stats_json: the scalar summary of one timeseries.
--
-- Null when the series holds no scalar values, which is every histogram series
-- -- they have their own totals and no meaningful mean of a bucket vector.
--
-- Computed over the whole window rather than over the datapoints a chart draws.
-- The client used to derive these from its chart points, which are thinned
-- before it sees them, so the average was the mean of an arbitrary sample and
-- the total was short by roughly the thinning factor.
create or replace macro series_stats_json(cnt, min_, max_, sum_) as (
		case when cnt > 0 then json_object(
			'count', cnt,
			'min', double_wire_json(min_),
			'max', double_wire_json(max_),
			'sum', double_wire_json(sum_),
			'avg', double_wire_json(sum_ / cnt)
		) end
	)
