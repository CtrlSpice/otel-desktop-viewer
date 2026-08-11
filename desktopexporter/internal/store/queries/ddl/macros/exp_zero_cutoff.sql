-- exp_zero_cutoff: the highest bucket index wholly inside the zero region.
--
-- An exponential histogram's zero_threshold T says "values at or below T are
-- counted in zero_count, not in a bucket". When two histograms with different
-- thresholds merge, the merged threshold is the larger of the two, and every
-- bucket that now falls entirely below it has to be folded into zero_count.
-- This computes where that line falls; fold_below_cutoff does the folding.
--
-- Bucket i at scale s covers (base^i, base^(i+1)] with base = 2^(2^-s), so
-- bucket i is wholly inside the zero region when its upper bound is at or
-- below T:
--
--	2^((i+1) * 2^-s) <= T
--	(i+1) * 2^-s     <= log2(T)
--	i                <= log2(T) * 2^s - 1
--
-- giving floor(log2(T) * 2^s) - 1. (floor(x - 1) = floor(x) - 1 for integer 1,
-- so the two groupings agree.)
--
-- NULL when there is no zero region to fold into, which fold_below_cutoff
-- treats as "fold nothing".
--
-- This exists as a macro rather than as an expression inside whichever query
-- needs it because it is the one line here that is wrong silently: get it
-- wrong and counts move between zero_count and the first bucket, which shifts
-- every quantile without producing an error. Named, it can be tested against
-- the boundary directly -- see TestMacros_ExpZeroCutoff -- and it matches
-- expPositiveCutoff in histogram-merge.ts, which is the reference.
create or replace macro exp_zero_cutoff(zero_threshold, scale) as (
    case
        when zero_threshold is null or zero_threshold <= 0 then null
        else cast(floor(log2(zero_threshold) * pow(2, scale)) as bigint) - 1
    end
)
