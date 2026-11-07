-- bucket_extents: the value range a set of histogram buckets actually covers.
--
-- Derive extents from populated buckets because cumulative merges cannot retain
-- received min/max. Empty input returns NULL. Open lower bounds become 0; open
-- upper bounds use their lower bound. A fully open bucket has no finite extent.
-- Keep this scalar: repeated correlated subqueries inside the JSON projection
-- cause excessive column-binding rewrites in DuckDB 2's planner.
create or replace macro bucket_extents(buckets) as (
    case
        when buckets is null or len(list_filter(buckets, lambda b: b.cnt > 0)) = 0 then null
        else {
            'min': list_min(list_transform(buckets,
                lambda b: case when b.cnt > 0 then case
                    when b.lo is null and b.hi is null then null
                    when isfinite(b.lo) then b.lo else 0.0 end end)),
            'max': list_max(list_transform(buckets,
                lambda b: case when b.cnt > 0 then case
                    when isfinite(b.hi) then b.hi else b.lo end end))
        }
    end
)
