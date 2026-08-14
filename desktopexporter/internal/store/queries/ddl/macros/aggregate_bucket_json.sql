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
create or replace macro aggregate_bucket_json(timestamp_, start_time, count_, sum_, scale, zero_threshold, zero_count, pos_fold, neg_fold, qs) as (
		json_object(
			'timestamp', timestamp_::varchar,
			'startTime', start_time::varchar,
			'count', count_,
			'sum', sum_,
			-- Derived from the buckets, because a merge cannot carry min and max
			-- through: for cumulative the merge is a subtraction, and you cannot
			-- subtract two minima to learn the minimum of the activity between
			-- them. bucket_extents says the same thing the client's bucketExtents
			-- does, from the same buckets.
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
	)
