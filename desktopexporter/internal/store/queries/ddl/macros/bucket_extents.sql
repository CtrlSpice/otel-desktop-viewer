-- bucket_extents: the value range a set of histogram buckets actually covers.
--
-- Merging histograms loses min and max. For delta they could in principle be
-- carried through (min of mins, max of maxes), but for cumulative they cannot:
-- the merge is a subtraction, and you cannot subtract two minima to learn the
-- minimum of the activity between them. So both paths derive the range from
-- the buckets that ended up holding counts, which is the same thing the client
-- does in bucketExtents (histogram-quantile.ts).
--
-- Empty buckets are skipped, so the range spans observed values rather than
-- the full bucket layout. NULL when nothing was observed at all.
--
-- The open ends are clamped rather than reported as infinite: an explicit
-- histogram's first bucket is (-inf, bounds[0]] and its last is
-- [bounds[n], +inf), and neither infinity is a value anything observed. A
-- missing lower bound becomes 0 -- these metrics are non-negative in practice
-- -- and a missing upper bound becomes that bucket's own lower bound, which is
-- the largest value the bucket can attest to.
create or replace macro bucket_extents(buckets) as (
    case
        when buckets is null then null
        else (
            select case
                when len(nonempty) = 0 then null
                else {
                    'min': list_min(list_transform(nonempty,
                        lambda b: case when isfinite(b.lo) then b.lo else 0.0 end)),
                    'max': list_max(list_transform(nonempty,
                        lambda b: case when isfinite(b.hi) then b.hi else b.lo end))
                }
            end
            from (select list_filter(buckets, lambda b: b.cnt > 0) as nonempty)
        )
    end
)
