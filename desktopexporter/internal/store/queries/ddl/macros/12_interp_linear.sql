-- Interpolation kernels.
-- interp_loglin falls back to linear when lo*hi <= 0 (zero endpoint or sign mismatch)
create or replace macro interp_linear(lo, hi, acc_prev, cnt, target) as (
		lo + (hi - lo) * (target - acc_prev) / cnt
	)
