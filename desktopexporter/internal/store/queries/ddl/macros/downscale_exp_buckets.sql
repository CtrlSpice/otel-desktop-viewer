-- Merge each 2^levels adjacent exponential buckets. Returns
-- {offset: bigint, counts: hugeint[]}; levels <= 0 is unchanged.
-- Output bucket k slices source indices [k*f, (k+1)*f), where f = 2^levels.
-- Keep the body free of subqueries: DuckDB cannot bind parameter-referencing
-- macro subqueries when the calling SELECT joins CTEs.
create or replace macro downscale_exp_buckets(counts, offset_, levels) as (
		case
			when levels <= 0
				then {'offset': offset_, 'counts': counts}
			-- Empty arrays still need a target-scale offset for later alignment.
			when counts is null or len(counts) = 0
				then {'offset': floor_div(offset_, cast(pow(2, levels) as bigint)), 'counts': counts}
			else {
				'offset': floor_div(offset_, cast(pow(2, levels) as bigint)),
				-- A derived bucket can exceed uint64, so retain HUGEINT.
				'counts': list_transform(
					range(
						0,
						floor_div(offset_ + len(counts) - 1, cast(pow(2, levels) as bigint))
							- floor_div(offset_, cast(pow(2, levels) as bigint))
							+ 1
					),
					-- list_slice is 1-based and inclusive. It does not clamp a
					-- lower bound below 1, so greatest() prevents dropped counts.
					lambda k_off: cast(
						coalesce(
							list_sum(
								list_slice(
									counts,
									greatest(
										(floor_div(offset_, cast(pow(2, levels) as bigint)) + k_off)
											* cast(pow(2, levels) as bigint) - offset_,
										0
									) + 1,
									least(
										(floor_div(offset_, cast(pow(2, levels) as bigint)) + k_off + 1)
											* cast(pow(2, levels) as bigint) - 1 - offset_,
										len(counts) - 1
									) + 1
								)
							),
							0
						)
						as hugeint
					)
				)
			}
		end
	)
