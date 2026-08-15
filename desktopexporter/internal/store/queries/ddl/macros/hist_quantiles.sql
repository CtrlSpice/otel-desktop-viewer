-- hist_quantiles: several quantiles of one explicit-bounds histogram, as a
-- JSON object keyed by the quantile.
--
-- Keys are `q::varchar`, which matches TypeScript's `String(q)` in
-- quantileRecord -- the two have to agree, because the client looks values up
-- by that key.
--
-- Exists so the store can answer "p50, p95 and p99 of this datapoint" in one
-- call. One call, not one pass: each quantile walks the buckets itself. Making
-- them share a single accumulation would need that accumulation as its own
-- macro, and a subquery-valued macro cannot be passed as an argument to
-- another -- see bucket_quantile_linear, which records the three ways that
-- fails and why the remaining cost is bounded.
--
-- A quantile with nothing to compute from yields null rather than an error, so
-- one empty datapoint does not void the whole object.
create or replace macro hist_quantiles(bounds, counts, qs) as (
		(select json_group_object(q::varchar, hist_quantile(bounds, counts, q))
		 from unnest(qs) t(q))
	)
