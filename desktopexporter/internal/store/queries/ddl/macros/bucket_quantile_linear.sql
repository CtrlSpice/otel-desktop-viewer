-- Shared quantile pipeline:
-- 1. params:    target = q * total
-- 2. with_acc:  attach acc_prev / acc to each bucket via list_transform with index
-- 3. chosen:    first bucket whose acc >= target
-- 4. interp:    apply linear or log-linear kernel
--
-- O(N^2) cumulative is fine for OTel histograms (N <= 160 buckets).
-- The two macros are intentionally identical except for the kernel call (option A:
-- explicit duplication beats runtime indirection through a strategy tag).
create or replace macro bucket_quantile_linear(buckets, q) as (
		case
			when buckets is null or len(buckets) = 0 then null
			when coalesce(list_sum(list_transform(buckets, lambda b: b.cnt)), 0) <= 0 then null
			else (
				with
					params as (
						select q * list_sum(list_transform(buckets, lambda b: b.cnt)) as target
					),
					with_acc as (
						select list_transform(buckets, lambda b, i: {
							'lo': b.lo, 'hi': b.hi, 'cnt': b.cnt,
							'acc_prev': case when i = 1 then 0
								else list_sum(list_transform(list_slice(buckets, 1, i - 1), lambda x: x.cnt))
							end,
							'acc': list_sum(list_transform(list_slice(buckets, 1, i), lambda x: x.cnt))
						}) as bs
					),
					chosen as (
						select
							params.target as target,
							-- cnt > 0 first: a quantile is never *inside* an empty
							-- bucket. For q > 0 this changes nothing, since an empty
							-- bucket's running total equals its predecessor's and the
							-- earlier bucket already satisfies the target. It matters
							-- only at q = 0, where target is 0 and the leading zero
							-- bucket that exp_buckets always emits would otherwise be
							-- selected with cnt = 0 -- the 0/0 the interp kernels now
							-- guard against.
							list_filter(with_acc.bs, lambda b: b.cnt > 0 and b.acc >= params.target)[1] as b
						from with_acc, params
					)
				select interp_linear(b.lo, b.hi, b.acc_prev, b.cnt, target) from chosen
			)
		end
	)
