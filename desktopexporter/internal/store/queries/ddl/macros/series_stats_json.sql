-- series_stats_json: the scalar summary of one timeseries.
-- NULL for histograms or series without scalar values. Inputs cover the full
-- window rather than the reduced chart sample.
create or replace macro series_stats_json(cnt, min_, max_, sum_) as (
		case when cnt > 0 then json_object(
			'count', cnt,
			'min', double_wire_json(min_),
			'max', double_wire_json(max_),
			'sum', double_wire_json(sum_),
			'avg', double_wire_json(sum_ / cnt)
		) end
	)
