-- aggregate_bucket_json: one time bucket of the cross-series histogram merge,
-- in wire shape.
--
-- Takes the folded positive and negative structs rather than their pieces, so
-- the caller passes what fold_below_cutoff returned instead of unpacking it
-- into six arguments and risking a transposition. Pure in its inputs: no table
-- reference, no correlated read, so it can be tested against literals the way
-- the interp kernels are.
--
-- zero_count arrives pre-fold and the folded totals are added here, in one
-- place. Written the other way round -- folding in the caller and passing the
-- total -- the addition would be repeated at every call site, and the wire
-- would depend on each one remembering it.
create or replace macro aggregate_bucket_json(timestamp_, start_time, count_, sum_, scale, zero_threshold, zero_count, pos_fold, neg_fold, bounds, counts, qs) as (
		case when bounds is not null and len(bounds) > 0 then
			-- Explicit bounds. The exponential fields stay out entirely rather than
			-- being emitted as nulls: a datapoint carries one representation or the
			-- other, and a reader should not have to work out which by probing.
			json_object(
				'timestamp', timestamp_::varchar,
				'startTime', start_time::varchar,
				'count', count_,
				'sum', sum_,
				'min', (bucket_extents(hist_buckets(bounds, counts))).min,
				'max', (bucket_extents(hist_buckets(bounds, counts))).max,
				'bucketCounts', counts,
				'explicitBounds', bounds,
				'quantiles', case when len(qs) = 0 then null
					else hist_quantiles(bounds, counts, qs) end
			)
		else
			json_object(
				'timestamp', timestamp_::varchar,
				'startTime', start_time::varchar,
				'count', count_,
				'sum', sum_,
				'min', (bucket_extents(exp_buckets(scale, neg_fold.offset, neg_fold.counts,
					zero_count + pos_fold.folded + neg_fold.folded,
					pos_fold.offset, pos_fold.counts))).min,
				'max', (bucket_extents(exp_buckets(scale, neg_fold.offset, neg_fold.counts,
					zero_count + pos_fold.folded + neg_fold.folded,
					pos_fold.offset, pos_fold.counts))).max,
				'scale', scale,
				'zeroThreshold', zero_threshold,
				'zeroCount', zero_count + pos_fold.folded + neg_fold.folded,
				'positiveBucketOffset', pos_fold.offset,
				'positiveBucketCounts', pos_fold.counts,
				'negativeBucketOffset', neg_fold.offset,
				'negativeBucketCounts', neg_fold.counts,
				'quantiles', case when len(qs) = 0 then null
					else exp_hist_quantiles(scale,
						neg_fold.offset, neg_fold.counts,
						zero_count + pos_fold.folded + neg_fold.folded,
						pos_fold.offset, pos_fold.counts, qs) end
			)
		end
	)
