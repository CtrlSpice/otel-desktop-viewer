-- Three-region concat in CDF order: most-negative -> zero -> most-positive.
-- Nested 2-arg list_concat for portability.
create or replace macro exp_buckets(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts) as (
		list_concat(
			list_concat(
				exp_neg_buckets(scale, neg_offset, neg_counts),
				exp_zero_bucket(zero_count)
			),
			exp_pos_buckets(scale, pos_offset, pos_counts)
		)
	)
