-- Element-wise a - b over aligned arrays; missing trailing elements are zero.
-- NULL signals a negative result, so callers can treat the later vector as a
-- reset. HUGEINT covers uint64 inputs and negative reset detection exactly.
create or replace macro diff_bucket_vectors(a, b) as (
    case
        when a is null then null
        when b is null then a
        -- Bind the difference once without a correlated scalar subquery.
        else list_transform(
            [list_transform(
                list_zip(a, b),
                lambda x: coalesce(x[1], 0)::hugeint - coalesce(x[2], 0)::hugeint
            )],
            lambda d: case when list_min(d) < 0 then null else d end
        )[1]
    end
)
