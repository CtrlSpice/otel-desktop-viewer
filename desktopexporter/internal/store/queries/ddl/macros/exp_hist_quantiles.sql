-- exp_hist_quantiles: several quantiles of one exponential histogram, as a
-- JSON object keyed by the quantile. See hist_quantiles for why this shape.
--
-- Must be created after exp_hist_quantile: DuckDB binds a macro body when the
-- macro is created, so the _order manifest places it later rather than leaving
-- it to alphabetical.
create or replace macro exp_hist_quantiles(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, qs) as (
		(select json_group_object(q::varchar,
			exp_hist_quantile(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, q))
		 from unnest(qs) t(q))
	)
