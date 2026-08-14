-- Interpolation kernels.
-- interp_loglin falls back to linear when lo*hi <= 0 (zero endpoint or sign mismatch)
--
-- cnt = 0 returns lo rather than dividing. An empty bucket contains no
-- observations, so there is no position within it to interpolate to, and the
-- bucket's own lower edge is the only defensible answer.
--
-- This is reachable, not defensive: exp_buckets emits a zero bucket
-- {lo: 0, hi: 0, cnt: 0} even when zero_count is 0, and at q = 0 the target is
-- 0, so the running total of that empty leading bucket already satisfies
-- acc >= target and it gets selected. Without the guard the expression is
-- 0 + (0 - 0) * (0 - 0) / 0 -- a plain division by zero that surfaced as a NaN
-- quantile, where the TypeScript implementation returned 0.
create or replace macro interp_linear(lo, hi, acc_prev, cnt, target) as (
		case
			when cnt is null or cnt = 0 then lo
			else lo + (hi - lo) * (target - acc_prev) / cnt
		end
	)
