-- pad_left_to_offset: left-pads `counts` with zeros so the first bucket
-- lines up with `target_offset`. Used during cross-stream exp-histogram
-- alignment after downscaling: every stream is downscaled to the group's
-- minimum scale, then padded so every aligned bucket array starts at the
-- same (minimum) offset.
--
-- Caller invariant is target_offset <= current_offset (you can only ever
-- extend a bucket array's coverage downward, never trim it). When the
-- invariant is violated or padding is unnecessary (target == current),
-- returns counts unchanged. NULL counts pass through.
--
-- Implementation note: DuckDB doesn't have list_repeat(value, n) in this
-- version, so the zero prefix is built via list_transform(range(0, n)).
-- The 0::hugeint cast keeps the prefix type aligned with widened derived
-- counts so list_concat does not narrow them back into a source-sized domain.
create or replace macro pad_left_to_offset(counts, current_offset, target_offset) as (
		case
			when counts is null or current_offset <= target_offset then counts
			else list_concat(
				list_transform(range(0, current_offset - target_offset), lambda x: 0::hugeint),
				counts
			)
		end
	)
