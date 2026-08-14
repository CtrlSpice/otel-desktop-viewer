create or replace macro bucket_quantile_loglin(buckets, q) as (
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
							list_filter(with_acc.bs, lambda b: b.acc >= params.target)[1] as b
						from with_acc, params
					)
				select interp_loglin(b.lo, b.hi, b.acc_prev, b.cnt, target) from chosen
			)
		end
	)
