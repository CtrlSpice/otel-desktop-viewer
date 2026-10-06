-- metric_datapoint_view_json: one datapoint in wire shape, whatever its metric type.
--
-- Exemplars and quantiles are arguments to avoid correlated table reads.
-- Histogram branches preserve NULL received optional statistics.
create or replace macro metric_datapoint_view_json(d, exemplars, exemplar_count, quantiles) as (
		-- exemplarCount is absent unless exemplars were withheld. RFC 7386
		-- removes the key when this patch value is NULL.
		json_merge_patch(
		case d.metric_type
			-- Complete objects retain NULL received optional statistics.
			when 'Histogram' then json_merge_patch(json_object(
				'id', d.id,
				'metricType', d.metric_type,
				'timestamp', d.timestamp::varchar,
				'timestampMs', d.timestamp // 1000000,
				'startTime', d.start_time::varchar,
				'flags', d.flags,
				'exemplars', exemplars,
				-- Received count vectors are unsigned uint64. Reduced histogram
				-- rows reuse this shape, so their integral results also retain
				-- their exact SQL value on the wire.
				'count', d.count::varchar,
				'sum', double_wire_json(d.sum),
				'min', double_wire_json(d.min),
				'max', double_wire_json(d.max),
				'bucketCounts', list_transform(d.bucket_counts, value -> value::varchar),
				'explicitBounds', list_transform(d.explicit_bounds, value -> double_wire_json(value)),
				'aggregationTemporalityCode', d.aggregation_temporality,
				'aggregationTemporality', case d.aggregation_temporality
					when 0 then 'Unspecified' when 1 then 'Delta' when 2 then 'Cumulative'
					else 'Unknown (' || d.aggregation_temporality::varchar || ')' end
			), json_object(
				-- Add quantiles without deleting NULL received statistics.
				'quantiles', json_merge_patch(json('{}'), quantiles)
			))
			when 'ExponentialHistogram' then json_merge_patch(json_object(
				'id', d.id,
				'metricType', d.metric_type,
				'timestamp', d.timestamp::varchar,
				'timestampMs', d.timestamp // 1000000,
				'startTime', d.start_time::varchar,
				'flags', d.flags,
				'exemplars', exemplars,
				'count', d.count::varchar,
				'sum', double_wire_json(d.sum),
				'min', double_wire_json(d.min),
				'max', double_wire_json(d.max),
				'scale', d.scale,
				'zeroCount', d.zero_count::varchar,
				'zeroThreshold', double_wire_json(d.zero_threshold),
				'positiveBucketOffset', d.positive_bucket_offset,
				'positiveBucketCounts', list_transform(d.positive_bucket_counts, value -> value::varchar),
				'negativeBucketOffset', d.negative_bucket_offset,
				'negativeBucketCounts', list_transform(d.negative_bucket_counts, value -> value::varchar),
				'aggregationTemporalityCode', d.aggregation_temporality,
				'aggregationTemporality', case d.aggregation_temporality
					when 0 then 'Unspecified' when 1 then 'Delta' when 2 then 'Cumulative'
					else 'Unknown (' || d.aggregation_temporality::varchar || ')' end
			), json_object(
				'quantiles', json_merge_patch(json('{}'), quantiles)
			))
			else json_merge_patch(
				json_object(
					'id', d.id,
					'metricType', d.metric_type,
					'timestamp', d.timestamp::varchar,
					-- Epoch milliseconds remain exact in float64.
					'timestampMs', d.timestamp // 1000000,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', exemplars
				),
				case d.metric_type
				when 'Gauge' then json_object(
					'doubleValue', double_wire_json(d.double_value),
					-- Decimal text preserves received signed int64 exactly.
					'intValue', d.int_value::varchar,
					'valueType', d.value_type
				)
				when 'Sum' then json_object(
					'doubleValue', double_wire_json(d.double_value),
					'intValue', d.int_value::varchar,
					'valueType', d.value_type,
					'isMonotonic', d.is_monotonic,
					'aggregationTemporalityCode', d.aggregation_temporality,
					'aggregationTemporality', case d.aggregation_temporality
						when 0 then 'Unspecified' when 1 then 'Delta' when 2 then 'Cumulative'
						else 'Unknown (' || d.aggregation_temporality::varchar || ')' end,
					-- Cumulative activity since the previous reading; NULL for the
					-- first reading. Delta values already represent interval activity.
					'delta', case when d.aggregation_temporality = 2 then
						case when d.delta_int is not null
							then to_json(d.delta_int::varchar)
							else double_wire_json(d.delta_double)
						end
					end,
					'isReset', case when d.aggregation_temporality = 2
						then d.is_reset end
				)
				end
			)
		end,
		json_object('exemplarCount',
			case when exemplar_count > json_array_length(exemplars)
				then exemplar_count end)
		)
	)
