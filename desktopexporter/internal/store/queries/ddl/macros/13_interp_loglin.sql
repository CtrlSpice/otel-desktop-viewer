create or replace macro interp_loglin(lo, hi, acc_prev, cnt, target) as (
		case
			when lo = 0 or hi = 0 or sign(lo) <> sign(hi)
				then interp_linear(lo, hi, acc_prev, cnt, target)
			else lo * pow(hi / lo, (target - acc_prev) / cnt)
		end
	)
