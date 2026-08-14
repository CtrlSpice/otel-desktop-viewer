-- Top-level convenience macros. All NULL/empty guards live here so callers
-- just see "give me a quantile, get null if it can't be computed".
create or replace macro hist_quantile(bounds, counts, q) as (
		case
			when bounds is null or counts is null or len(bounds) = 0 or len(counts) = 0 then null
			else bucket_quantile_linear(hist_buckets(bounds, counts), q)
		end
	)
