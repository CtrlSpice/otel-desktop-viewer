-- zero_fold: reconcile one bucket array with a zero threshold.
--
-- Returns {counts, offset, folded}; callers add folded to zero_count.
-- NULL and non-positive thresholds leave explicit histograms unchanged.
create or replace macro zero_fold(counts, offset_, zero_threshold, scale) as (
		fold_below_cutoff(counts, offset_, exp_zero_cutoff(zero_threshold, scale))
	)
