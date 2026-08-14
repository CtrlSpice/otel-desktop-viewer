-- Log-linear interpolation within a bucket, for exponential histograms whose
-- bucket boundaries are geometric rather than evenly spaced.
--
-- cnt = 0 returns lo before anything else is evaluated: the exponential branch
-- divides by cnt too (pow(hi/lo, (target - acc_prev) / cnt)), so deferring to
-- interp_linear's guard would only cover half the cases. See interp_linear for
-- why an empty bucket is reachable here at all.
--
-- Falls back to linear when lo * hi <= 0, i.e. a zero endpoint or a sign
-- mismatch, because pow() cannot span zero.
create or replace macro interp_loglin(lo, hi, acc_prev, cnt, target) as (
		case
			when cnt is null or cnt = 0 then lo
			when lo = 0 or hi = 0 or sign(lo) <> sign(hi)
				then interp_linear(lo, hi, acc_prev, cnt, target)
			else lo * pow(hi / lo, (target - acc_prev) / cnt)
		end
	)
