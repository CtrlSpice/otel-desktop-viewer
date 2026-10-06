-- Fold buckets with index <= cutoff into {counts, offset, folded}.
-- The following inputs return unchanged counts/offset and folded = 0:
-- - counts is NULL or empty
-- - cutoff is NULL
-- - cutoff < offset_
-- list_slice is 1-based; the cutoff guard keeps its start positive.
create or replace macro fold_below_cutoff(counts, offset_, cutoff) as (
		case
			when counts is null or len(counts) = 0 or cutoff is null or cutoff < offset_
				then {'counts': counts, 'offset': offset_, 'folded': 0::hugeint}
			-- Repeat drop_n because DuckDB forbids subqueries in lambda expressions.
			else {
				'counts': list_slice(counts, least(cutoff - offset_ + 1, len(counts)) + 1, len(counts)),
				'offset': offset_ + least(cutoff - offset_ + 1, len(counts)),
				'folded': cast(coalesce(list_sum(list_slice(counts, 1, least(cutoff - offset_ + 1, len(counts)))), 0) as hugeint)
			}
		end
	)
