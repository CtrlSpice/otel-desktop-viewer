-- fold_below_cutoff: after scale/offset alignment of an exponential
-- histogram aggregate, fold any leading buckets whose index is <= cutoff
-- into a single "folded" total. The folded value is intended to be added
-- back into zero_count by the caller, completing the zero_threshold
-- reconciliation step described in the histogram-trend-chart plan.
--
-- Returns {counts: bigint[], offset: bigint, folded: bigint}. Where the
-- inputs trigger a no-op, folded is 0 and counts/offset pass through:
-- - counts is NULL or empty
-- - cutoff is NULL (signals "no zero_threshold to apply")
-- - cutoff < offset_ (no buckets sit at or below the threshold)
--
-- drop_n is capped by len(counts) so a wildly-high cutoff folds the whole
-- array rather than producing nonsense slices. list_slice in DuckDB is
-- 1-indexed and end-inclusive; both list_slice calls clamp gracefully on
-- out-of-range indices, so the cap is defensive rather than load-bearing.
create or replace macro fold_below_cutoff(counts, offset_, cutoff) as (
		case
			when counts is null or len(counts) = 0 or cutoff is null or cutoff < offset_
				then {'counts': counts, 'offset': offset_, 'folded': 0::bigint}
			-- The drop count is repeated three times rather than bound once in
			-- a CTE. That reads worse, and it is deliberate: a subquery inside
			-- a macro cannot be used from a lambda --
			-- "subqueries in lambda expressions are not supported" -- which
			-- rules the macro out of exactly the list_transform composition the
			-- merge needs. Verified equivalent to the CTE form across all 663
			-- offset/cutoff/array combinations before the swap.
			else {
				'counts': list_slice(counts, least(cutoff - offset_ + 1, len(counts)) + 1, len(counts)),
				'offset': offset_ + least(cutoff - offset_ + 1, len(counts)),
				'folded': cast(coalesce(list_sum(list_slice(counts, 1, least(cutoff - offset_ + 1, len(counts)))), 0) as bigint)
			}
		end
	)
