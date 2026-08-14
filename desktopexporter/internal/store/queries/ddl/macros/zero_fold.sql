-- zero_fold: reconcile one bucket array with a zero threshold.
--
-- Composes the two halves that always travel together -- exp_zero_cutoff finds
-- where the zero region ends at this scale, fold_below_cutoff moves everything
-- under it -- so a caller states the intent ("fold this array against this
-- threshold") rather than the mechanism.
--
-- Returns fold_below_cutoff's {counts, offset, folded} unchanged, because the
-- caller still has to add `folded` into zero_count and that addition belongs
-- where zero_count lives.
--
-- Both merges need this: the per-series merge takes the largest threshold of a
-- time bucket, the cross-series merge takes the largest across the selected
-- series, and either way the input with the smaller threshold is left holding
-- buckets the merged threshold declares empty. Four call sites before this
-- existed, each spelling out the same composition.
--
-- Explicit-bounds histograms pass through untouched: they carry no zero
-- threshold, exp_zero_cutoff returns NULL for a null or non-positive one, and
-- fold_below_cutoff treats a NULL cutoff as "fold nothing".
create or replace macro zero_fold(counts, offset_, zero_threshold, scale) as (
		fold_below_cutoff(counts, offset_, exp_zero_cutoff(zero_threshold, scale))
	)
