-- datapoint_json: one datapoint in wire shape, whatever its metric type.
--
-- Takes the row as a struct, so the caller passes `d` rather than sixteen
-- columns in an order that has to stay right. exemplars and qs arrive as
-- arguments instead of being read inside: exemplars is a correlated lookup and
-- qs comes from the input CTE, and a macro that reaches for a table binds that
-- reference when the macro is created -- which couples macro creation order to
-- table creation order and is what ruled out attr_dict as a table macro.
--
-- Keeping both out means this is a pure function of its arguments, testable
-- against literals, and the two things it cannot know stay the caller's job.
--
-- The common fields are merged with the type-specific ones rather than
-- repeated in each branch, so a field every datapoint carries is written once.
create or replace macro datapoint_json(d, exemplars, exemplar_count, qs) as (
		json_merge_patch(
			json_object(
				'id', d.id,
				'metricType', d.metric_type,
				'timestamp', d.timestamp::varchar,
				-- The same instant in epoch milliseconds, as a number.
				--
				-- The chart needs milliseconds and got them by dividing the
				-- nanosecond string's BigInt per datapoint -- 23,000 BigInt
				-- divisions to draw one Gauge. Epoch ms is ~1.8e12, comfortably
				-- inside float64's exact-integer range, so unlike the ns value it
				-- loses nothing as a JSON number.
				'timestampMs', d.timestamp // 1000000,
				'startTime', d.start_time::varchar,
				'flags', d.flags,
				-- How many exemplars this datapoint holds, which is not always
				-- how many arrived: the list is capped so one aggressively
				-- sampled stream cannot decide the size of the response.
				'exemplarCount', exemplar_count,
				'exemplars', exemplars
			),
			case d.metric_type
				when 'Gauge' then json_object(
					'doubleValue', d.double_value,
					'intValue', d.int_value,
					'valueType', d.value_type
				)
				when 'Sum' then json_object(
					'doubleValue', d.double_value,
					'intValue', d.int_value,
					'valueType', d.value_type,
					'isMonotonic', d.is_monotonic,
					'aggregationTemporality', d.aggregation_temporality,
					-- Activity since the previous reading of this series, and
					-- whether the counter restarted in between. Null on the first
					-- datapoint of a series, which describes no interval.
					--
					-- Cumulative only: a Delta Sum's value already *is* the
					-- interval's activity, so differencing it would be wrong.
					'delta', case when d.aggregation_temporality = 'Cumulative'
						then d.delta end,
					'isReset', case when d.aggregation_temporality = 'Cumulative'
						then d.is_reset end
				)
				when 'Histogram' then json_object(
					'count', d.count,
					'sum', d.sum,
					'min', d.min,
					'max', d.max,
					'bucketCounts', d.bucket_counts,
					'explicitBounds', d.explicit_bounds,
					'quantiles', case when len(qs) = 0 then null
						else hist_quantiles(d.explicit_bounds, d.bucket_counts, qs) end,
					'aggregationTemporality', d.aggregation_temporality
				)
				when 'ExponentialHistogram' then json_object(
					'count', d.count,
					'sum', d.sum,
					'min', d.min,
					'max', d.max,
					'scale', d.scale,
					'zeroCount', d.zero_count,
					'zeroThreshold', d.zero_threshold,
					'positiveBucketOffset', d.positive_bucket_offset,
					'positiveBucketCounts', d.positive_bucket_counts,
					'negativeBucketOffset', d.negative_bucket_offset,
					'negativeBucketCounts', d.negative_bucket_counts,
					'quantiles', case when len(qs) = 0 then null
						else exp_hist_quantiles(d.scale,
							d.negative_bucket_offset, d.negative_bucket_counts,
							d.zero_count,
							d.positive_bucket_offset, d.positive_bucket_counts, qs) end,
					'aggregationTemporality', d.aggregation_temporality
				)
			end
		)
	)
