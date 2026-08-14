-- hist_quantiles: several quantiles of one explicit-bounds histogram, as a
-- JSON object keyed by the quantile.
--
-- Keys are `q::varchar`, which matches TypeScript's `String(q)` in
-- quantileRecord -- the two have to agree, because the client looks values up
-- by that key.
--
-- Exists so the store can answer "p50, p95 and p99 of this datapoint" in one
-- pass. Computing quantiles here rather than shipping bucket vectors for the
-- client to reduce is what lets a response carry three numbers per series per
-- bucket instead of a forty-element array.
--
-- A quantile with nothing to compute from yields null rather than an error, so
-- one empty datapoint does not void the whole object.
create or replace macro hist_quantiles(bounds, counts, qs) as (
		(select json_group_object(q::varchar, hist_quantile(bounds, counts, q))
		 from unnest(qs) t(q))
	)
