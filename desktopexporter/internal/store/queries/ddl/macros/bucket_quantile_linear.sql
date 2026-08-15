-- Shared quantile pipeline:
-- 1. acc:    attach acc_prev / acc to each bucket, in one windowed pass
-- 2. chosen: first bucket whose acc >= q * total
-- 3. interp: apply the linear kernel
--
-- The accumulation used to be a list_transform where each bucket re-sliced and
-- re-summed every bucket before it:
--
--	list_sum(list_transform(list_slice(buckets, 1, i - 1), lambda x: x.cnt))
--
-- O(N^2) in list elements, under a comment calling that "fine for OTel
-- histograms (N <= 160 buckets)". Fine for one quantile of one datapoint, and
-- neither of those is what a response asks for: three quantiles across every
-- datapoint in the window. Measured on a 21-series exponential histogram
-- reduced to 400 buckets, the request took 31,623 ms with three quantiles
-- against 285 ms with none -- 111x, for a field the client then discarded.
--
-- A window over the unnested list does the same work once per bucket: sum over
-- the frame ending one row back for acc_prev, at the current row for acc.
--
-- It stays one macro rather than splitting the accumulation into a reusable
-- bucket_acc, which would let several quantiles of one datapoint share a
-- single pass. That split is not expressible here: a macro returning a
-- subquery cannot be passed as an argument to another macro, because DuckDB
-- rejects a subquery in a lambda, in a table function, and -- when bound
-- through a CTE instead -- quietly shadows the caller's FROM scope, so
-- `unnest(qs) t(q)` in hist_quantiles stops resolving. All three were tried.
-- Repeating an O(N) pass per quantile is the cost of that; it is three passes
-- rather than one, against the triangle it replaces.
--
-- The alias is `bqlx`, not `t`: this body is inlined inside hist_quantiles,
-- which walks its quantile list with `unnest(qs) t(q)`. Two `t`s in one
-- inlined body bind to each other, and DuckDB reports the failure against a
-- line in an unrelated CTE.
--
-- acc_prev coalesces because the first row's frame is empty and an empty SUM
-- is NULL, which would otherwise feed NULL into interpolation for the first
-- bucket -- the one q = 0 lands in.
--
-- cnt > 0 comes before the acc test: a quantile is never *inside* an empty
-- bucket. For q > 0 it changes nothing, since an empty bucket's running total
-- equals its predecessor's and the earlier bucket already satisfied the
-- target. It matters at q = 0, where target is 0 and the leading zero bucket
-- exp_buckets always emits would otherwise be chosen with cnt = 0 -- the 0/0
-- the interp kernels guard against.
--
-- The pair is intentionally identical except for the kernel: explicit
-- duplication beats runtime indirection through a strategy tag.
create or replace macro bucket_quantile_linear(buckets, q) as (
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
					from unnest(buckets) with ordinality as bqlx(b, i)
				),
				total as (select max(acc) as n from acc)
				select interp_linear(acc.lo, acc.hi, acc.acc_prev, acc.cnt, q * total.n)
				from acc, total
				where acc.cnt > 0 and acc.acc >= q * total.n
				order by acc.i
				limit 1
			)
		end
	)
