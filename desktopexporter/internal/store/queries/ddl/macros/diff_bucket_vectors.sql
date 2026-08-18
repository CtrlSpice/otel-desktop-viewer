-- diff_bucket_vectors: element-wise a - b over two aligned bucket arrays.
--
-- The cumulative counterpart to sum_bucket_vectors. A cumulative histogram
-- datapoint is a running total, so the activity within a time bucket is the
-- last datapoint minus the first -- not the sum, which would multiply-count
-- everything.
--
-- Both arrays must already share an origin; align with downscale_exp_buckets
-- and pad_left_to_offset first. Missing trailing elements read as zero, so a
-- bucket present in one and absent in the other is treated as the zero it is.
--
-- Returns NULL if any element would go negative. That is the counter-reset
-- signal: the caller falls back to the later slice, because after a restart
-- the later value *is* the activity. Distinguishing "reset" from "could not
-- align" matters -- conflating them reports a running total as though it were
-- a delta.
--
-- Both sides are cast to a signed type before subtracting, because bucket
-- counts are UBIGINT and an unsigned subtraction raises rather than going
-- negative: "Overflow in subtraction of UINT64 (0 - 1)". The negative check
-- below could therefore never fire on the very inputs it exists for -- a reset
-- came back as a query error instead of a fallback. DuckDB also evaluates both
-- arms of the surrounding CASE, so this was reachable for Delta histograms
-- too, which never wanted a difference at all.
create or replace macro diff_bucket_vectors(a, b) as (
    case
        when a is null then null
        when b is null then a
        else (
            select case when list_min(d) < 0 then null else d end
            from (select list_transform(
                list_zip(a, b),
                lambda x: coalesce(x[1], 0)::bigint - coalesce(x[2], 0)::bigint
            ) as d)
        )
    end
)
