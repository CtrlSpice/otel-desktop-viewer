-- datapoint_json: one datapoint in wire shape, whatever its metric type.
--
-- Takes the row as a struct, so the caller passes `d` rather than sixteen
-- columns in an order that has to stay right. exemplars and quantiles arrive as
-- arguments instead of being read inside: exemplars is a correlated lookup and
-- quantiles is a correlated per-row value, and a macro that reaches for a table binds that
-- reference when the macro is created -- which couples macro creation order to
-- table creation order and is what ruled out attr_dict as a table macro.
--
-- Keeping both out means this is a pure function of its arguments, testable
-- against literals, and the two things it cannot know stay the caller's job.
--
-- Gauge and Sum merge common fields with their type-specific fields. Histogram
-- branches are complete objects because their optional nulls must survive.
create or replace macro datapoint_json(d, exemplars, exemplar_count, quantiles) as (
		-- exemplarCount rides in an outer patch rather than the object below,
		-- so that it can be *absent* rather than zero.
		--
		-- It answers "were any exemplars withheld", and the answer is no for
		-- almost every datapoint that will ever exist -- the reference corpus
		-- contains no exemplars at all. Emitting it unconditionally cost 6.6% of
		-- a zero-exemplar response, which is a poor trade in a change whose
		-- whole subject is payload. A null in a merge patch deletes the key
		-- (RFC 7386), so the common case pays nothing and the client reads
		-- absence as "you have them all".
		json_merge_patch(
		case d.metric_type
			-- These branches are complete objects rather than merge patches:
			-- RFC 7386 deletes null patch members, while null here is the wire
			-- representation of an absent received optional statistic.
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
				-- This patch preserves the existing quantile wire rule without
				-- touching null received statistics already in the target object.
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
					-- The same instant in epoch milliseconds, as a number. Epoch ms
					-- remains inside float64's exact-integer range.
					'timestampMs', d.timestamp // 1000000,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', exemplars
				),
				case d.metric_type
				when 'Gauge' then json_object(
					'doubleValue', double_wire_json(d.double_value),
					-- Received NumberDataPoint.as_int is signed int64. Decimal text
					-- preserves its exact value through JSON; the frontend revives it
					-- to bigint before any display-only chart projection.
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
					-- Activity since the previous reading of this series, and
					-- whether the counter restarted in between. Null on the first
					-- datapoint of a series, which describes no interval.
					--
					-- Cumulative only: a Delta Sum's value already *is* the
					-- interval's activity, so differencing it would be wrong.
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
