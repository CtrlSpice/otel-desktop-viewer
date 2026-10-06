-- metric_aggregate_bucket_view_json: one time bucket of the cross-series histogram merge,
-- in wire shape.
--
-- pos_fold and neg_fold are {counts, offset, folded}; folded totals are added
-- to zero_count here.
create or replace macro metric_aggregate_bucket_view_json(timestamp_, start_time, count_, sum_, scale, zero_threshold, zero_count, pos_fold, neg_fold, bounds, counts, quantiles) as (
		case when bounds is not null then
			-- Non-NULL bounds identify explicit histograms; an empty list is valid.
			json_merge_patch(json_object(
				'timestamp', timestamp_::varchar,
				'startTime', start_time::varchar,
				'count', count_,
				'sum', double_wire_json(sum_),
				'bucketCounts', counts,
				'explicitBounds', list_transform(bounds, value -> double_wire_json(value)),
				-- NULL when no quantiles were requested.
				'quantiles', quantiles
			), json_object(
				-- Remove derived extents when no populated bucket provides one.
				'min', double_wire_json((bucket_extents(hist_buckets(bounds, counts))).min),
				'max', double_wire_json((bucket_extents(hist_buckets(bounds, counts))).max)
			))
		else
			json_merge_patch(json_object(
				'timestamp', timestamp_::varchar,
				'startTime', start_time::varchar,
				'count', count_,
				'sum', double_wire_json(sum_),
				'scale', scale,
				'zeroThreshold', double_wire_json(zero_threshold),
				'zeroCount', zero_count + pos_fold.folded + neg_fold.folded,
				'positiveBucketOffset', pos_fold.offset,
				'positiveBucketCounts', pos_fold.counts,
				'negativeBucketOffset', neg_fold.offset,
				'negativeBucketCounts', neg_fold.counts,
				'quantiles', quantiles
			), json_object(
				'min', double_wire_json((bucket_extents(exp_buckets(scale, neg_fold.offset, neg_fold.counts,
					zero_count + pos_fold.folded + neg_fold.folded,
					pos_fold.offset, pos_fold.counts))).min),
				'max', double_wire_json((bucket_extents(exp_buckets(scale, neg_fold.offset, neg_fold.counts,
					zero_count + pos_fold.folded + neg_fold.folded,
					pos_fold.offset, pos_fold.counts))).max)
			))
		end
	)
