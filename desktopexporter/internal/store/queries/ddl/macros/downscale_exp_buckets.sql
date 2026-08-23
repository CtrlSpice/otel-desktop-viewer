-- downscale_exp_buckets: drop the resolution of an exponential histogram
-- by `levels` scale steps. A single "level" merges every pair of adjacent
-- buckets; level k merges 2^k adjacent buckets. Used during cross-stream
-- aggregation when streams arrive at different scales -- everyone gets
-- downscaled to the group's minimum scale before bucket-wise summation.
--
-- Returns {offset: bigint, counts: bigint[]}. levels <= 0 (and null/empty
-- counts) is a no-op: input is returned unchanged. Negative levels would
-- require *upscaling*, which is not generally possible without losing
-- information about the original sub-bucket distribution.
--
-- Approach: each output bucket's inputs are a contiguous run of positions,
-- so take them by slice rather than by search.
--
-- The mapping position -> output bucket, floor_div(offset_ + p, 2^levels),
-- is monotonically non-decreasing in p. That means the inputs feeding one
-- output bucket are always adjacent, and their bounds are arithmetic:
-- output bucket base+k covers original indices [(base+k)*f, (base+k+1)*f)
-- for f = 2^levels, which is positions [(base+k)*f - offset_, ...] clamped
-- to the array. list_slice takes them in one go.
--
-- The previous formulation filtered the whole input list once per output
-- bucket -- list_zip to pair counts with positions, then list_filter to
-- find the ones belonging to bucket k. Correct, but O(outputs x inputs),
-- and outputs scale with inputs, so it was quadratic in bucket count:
-- measured 8ms / 24ms / 83ms / 322ms / 1.27s at 20 / 40 / 80 / 160 / 320
-- buckets over 2,000 rows, quadrupling per doubling. The slice form is
-- 2ms / 3ms / 7ms / 20ms / 69ms on the same shapes -- 5x at 20 buckets,
-- 18x at 320, and the gap widens.
--
-- Verified against the previous implementation rather than reasoned about:
-- 4,000 randomised (length, offset, levels) triples including negative
-- offsets, plus the edge cases below, all byte-identical.
-- Implementation note: the macro body must NOT contain a subquery (no
-- `with`, no `select`). DuckDB refuses to bind subqueries that reference
-- macro parameters when the macro is called from a SELECT that itself
-- joins CTEs -- you get "Referenced table X not found! Candidate tables:
-- params". So the helper values factor / new_offset / last_k get
-- inlined; verbose but the planner is happy. Each subexpression is pure
-- arithmetic on the macro's parameters, so DuckDB folds the duplicates.
create or replace macro downscale_exp_buckets(counts, offset_, levels) as (
		case
			-- Nothing to rescale to: the offset is already at the target scale.
			when levels <= 0
				then {'offset': offset_, 'counts': counts}
			-- Empty counts still carry an offset, and that offset is expressed
			-- at the *source* scale. Returning it unchanged leaks a source-scale
			-- index into a target-scale comparison: the caller takes min() over
			-- every stream's offset to find the alignment point, so one empty
			-- high-scale array drags the target offset far below where any real
			-- bucket sits, and pad_left_to_offset then zero-fills out to it.
			-- Numerically harmless, unbounded in memory.
			when counts is null or len(counts) = 0
				then {'offset': floor_div(offset_, cast(pow(2, levels) as bigint)), 'counts': counts}
			else {
				'offset': floor_div(offset_, cast(pow(2, levels) as bigint)),
				-- list_sum promotes to HUGEINT; cast back to BIGINT so the
				-- output type matches the input and downstream macros that
				-- expect bigint[] (sum_bucket_vectors, exp_pos_buckets, ...)
				-- don't trip on inferred-type mismatches.
				'counts': list_transform(
					range(
						0,
						floor_div(offset_ + len(counts) - 1, cast(pow(2, levels) as bigint))
							- floor_div(offset_, cast(pow(2, levels) as bigint))
							+ 1
					),
					-- list_slice is 1-based and inclusive, hence the + 1 on both
					-- bounds. Only the first and last output buckets can be
					-- partial, so those are the only ones the clamps touch.
					--
					-- The two clamps are not equally load-bearing, which is
					-- worth knowing before anyone tidies one away. list_slice
					-- clamps an over-long upper bound itself -- slice([1,2,3],
					-- 1, 99) is [1,2,3] -- so `least` is belt-and-braces. A
					-- lower bound below 1 is *not* clamped: slice([1,2,3], -1,
					-- 2) returns [], silently dropping the counts. `greatest`
					-- is what stops the first bucket doing that whenever
					-- offset_ does not sit on a 2^levels boundary.
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
						as bigint
					)
				)
			}
		end
	)
