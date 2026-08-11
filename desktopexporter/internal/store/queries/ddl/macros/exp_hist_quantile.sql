create or replace macro exp_hist_quantile(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, q) as (
		bucket_quantile_loglin(
			exp_buckets(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts),
			q
		)
	)
