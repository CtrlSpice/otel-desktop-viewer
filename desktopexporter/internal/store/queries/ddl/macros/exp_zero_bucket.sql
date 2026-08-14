-- Zero bucket: always emit one entry to keep list_concat type-stable.
-- A zero-count entry is harmless: the filter step skips it (acc doesn't change).
create or replace macro exp_zero_bucket(zero_count) as (
		[{'lo': 0.0, 'hi': 0.0, 'cnt': coalesce(zero_count, 0)}]
	)
