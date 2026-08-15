-- exp_hist_quantiles: several quantiles of one exponential histogram, keyed by
-- the quantile, matching hist_quantiles' shape and key convention.
--
-- Each quantile rebuilds the bucket list from scale, offsets and count vectors
-- and walks it, which is the same repetition hist_quantiles documents and is
-- unavoidable for the same reason: the shared work cannot be lifted into a
-- macro of its own, because a subquery-valued macro is not passable as an
-- argument. What the repetition is no longer paying for is the quadratic
-- accumulation inside each walk -- see bucket_quantile_linear.
--
-- Must be created after exp_hist_quantile: DuckDB binds a macro body when the
-- macro is created, so the _order manifest places it later rather than leaving
-- it to alphabetical.
create or replace macro exp_hist_quantiles(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, qs) as (
		(select json_group_object(q::varchar,
			exp_hist_quantile(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, q))
		 from unnest(qs) t(q))
	)
