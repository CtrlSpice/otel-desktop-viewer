-- Linear bucket interpolation. Empty buckets return lo; q = 0 can select the
-- empty leading zero bucket emitted by exp_buckets.
-- Counts stay exact until this display-arithmetic boundary; target is DOUBLE.
create or replace macro interp_linear(lo, hi, acc_prev, cnt, target) as (
		case
			when cnt is null or cnt = 0 then lo
			else lo + (hi - lo) * (target - hugeint_to_double(acc_prev)) / hugeint_to_double(cnt)
		end
	)
