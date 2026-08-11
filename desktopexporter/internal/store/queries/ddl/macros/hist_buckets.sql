-- Bucket builders. Each emits a list of {lo, hi, cnt} structs in CDF walking order.
-- Cumulative counts are NOT computed here; bucket_quantile_* adds them.
-- Explicit-bound histogram. counts has len(bounds)+1 entries.
-- Open extreme buckets (i=1 and i=len(counts)) are clamped to bounds[1] / bounds[end]
-- so quantile interpolation in those regions returns the boundary value
-- (Prometheus convention; better than guessing an unbounded width).
create or replace macro hist_buckets(bounds, counts) as (
		list_transform(counts, lambda c, i: {
			'lo': case
					when i = 1 then bounds[1]
					when i = len(counts) then bounds[len(bounds)]
					else bounds[i - 1]
				  end,
			'hi': case
					when i = 1 then bounds[1]
					when i = len(counts) then bounds[len(bounds)]
					else bounds[i]
				  end,
			'cnt': c
		})
	)
