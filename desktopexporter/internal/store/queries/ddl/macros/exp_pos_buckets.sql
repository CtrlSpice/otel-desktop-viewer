-- Exponential histogram positive region. base = 2^(2^-scale).
-- Bucket at 1-based position i covers (base^(offset+i-1), base^(offset+i)].
create or replace macro exp_pos_buckets(scale, offset_, counts) as (
		list_transform(counts, lambda c, i: {
			'lo': pow(2.0, pow(2.0, -scale) * (offset_ + i - 1)),
			'hi': pow(2.0, pow(2.0, -scale) * (offset_ + i)),
			'cnt': c
		})
	)
