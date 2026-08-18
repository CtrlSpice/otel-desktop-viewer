-- Log-linear counterpart of bucket_quantile_linear, for exponential-histogram
-- buckets whose widths grow geometrically. See there for why the accumulation
-- is a window rather than a prefix re-sum, why it is not split into a shared
-- macro, and why the unnest alias is not `t`.
create or replace macro bucket_quantile_loglin(buckets, q) as (
		case
			when buckets is null or len(buckets) = 0 then null
			when coalesce(list_sum(list_transform(buckets, lambda b: b.cnt)), 0) <= 0 then null
			else (
				with acc as (
					select
						b.lo as lo, b.hi as hi, b.cnt as cnt, i as i,
						coalesce(sum(b.cnt) over (
							order by i rows between unbounded preceding and 1 preceding
						), 0) as acc_prev,
						sum(b.cnt) over (
							order by i rows between unbounded preceding and current row
						) as acc
					from unnest(buckets) with ordinality as bqgx(b, i)
				),
				total as (select max(acc) as n from acc)
				select interp_loglin(acc.lo, acc.hi, acc.acc_prev, acc.cnt, q * total.n)
				from acc, total
				where acc.cnt > 0 and acc.acc >= q * total.n
				order by acc.i
				limit 1
			)
		end
	)
