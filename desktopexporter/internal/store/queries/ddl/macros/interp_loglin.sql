-- Log-linear interpolation within a bucket, for exponential histograms whose
-- bucket boundaries are geometric rather than evenly spaced.
--
-- Empty buckets return lo before division. Zero endpoints and sign changes use
-- linear interpolation because pow() cannot span zero.
create or replace macro interp_loglin(lo, hi, acc_prev, cnt, target) as (
		case
			when cnt is null or cnt = 0 then lo
			when lo = 0 or hi = 0 or sign(lo) <> sign(hi)
				then interp_linear(lo, hi, acc_prev, cnt, target)
			else lo * pow(hi / lo, (target - acc_prev) / cnt)
		end
	)
