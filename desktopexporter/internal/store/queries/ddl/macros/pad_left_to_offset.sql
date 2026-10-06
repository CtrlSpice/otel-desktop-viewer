-- Left-pad counts to target_offset; target_offset must not exceed
-- current_offset. A HUGEINT zero prefix preserves widened derived counts.
create or replace macro pad_left_to_offset(counts, current_offset, target_offset) as (
		case
			when counts is null or current_offset <= target_offset then counts
			else list_concat(
				list_transform(range(0, current_offset - target_offset), lambda x: 0::hugeint),
				counts
			)
		end
	)
