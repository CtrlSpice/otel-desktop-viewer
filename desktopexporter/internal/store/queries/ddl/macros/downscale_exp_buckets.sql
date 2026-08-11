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
-- Approach: pair each input count with its 0-based position via list_zip,
-- then for each output bucket k in [new_offset, last_k] keep the inputs
-- whose original bucket index (offset_ + position) maps to k under
-- floor_div, and sum their counts. Single allocation per output bucket.
--
-- Note on list_zip pair access: list_zip returns structs that DuckDB
-- treats as "unnamed" for .field access -- you have to index positionally
-- (pair[1], pair[2]) the same way sum_bucket_vectors does. The fields are
-- 1=count, 2=0-based position.
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
					lambda k_off: cast(
						coalesce(
							list_sum(
								list_transform(
									list_filter(
										list_zip(counts, range(0, len(counts))),
										lambda pair: floor_div(offset_ + pair[2], cast(pow(2, levels) as bigint))
											= floor_div(offset_, cast(pow(2, levels) as bigint)) + k_off
									),
									lambda pair: pair[1]
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
