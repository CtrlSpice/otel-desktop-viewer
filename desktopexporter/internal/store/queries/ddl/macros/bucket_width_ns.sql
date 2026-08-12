-- bucket_width_ns: the time-bucket width to reduce a window to, in nanoseconds.
--
-- Picks the smallest rung of a fixed ladder that divides the window into at
-- most `target_buckets` buckets. The ladder is the one the histogram path
-- already uses (BUCKET_LADDER in histogram-aggregation.ts) so both reductions
-- land on the same boundaries.
--
-- The ladder exists to keep boundaries *stable*. Bucket starts are absolute --
-- floor(timestamp / width) * width -- not measured from the window start, so
-- panning slides data through fixed buckets instead of re-cutting them. If the
-- width were simply span/target it would change with every pan and every point
-- would move; snapping to a rung means only crossing a rung changes anything,
-- which is a visible step rather than continuous churn. Every rung divides its
-- next unit evenly, which is what makes that true.
--
-- Returns NULL when no reduction is wanted, which callers read as "return
-- every datapoint".
create or replace macro bucket_width_ns(span_ns, target_buckets) as (
    case
        when target_buckets is null or target_buckets <= 0 or span_ns <= 0 then null
        else cast(coalesce(
            list_min(list_filter(
                [
                    1000000::bigint,          -- 1ms
                    10000000::bigint,         -- 10ms
                    100000000::bigint,        -- 100ms
                    250000000::bigint,        -- 250ms
                    500000000::bigint,        -- 500ms
                    1000000000::bigint,       -- 1s
                    5000000000::bigint,       -- 5s
                    10000000000::bigint,      -- 10s
                    30000000000::bigint,      -- 30s
                    60000000000::bigint,      -- 1m
                    300000000000::bigint,     -- 5m
                    600000000000::bigint,     -- 10m
                    900000000000::bigint,     -- 15m
                    1800000000000::bigint,    -- 30m
                    3600000000000::bigint,    -- 1h
                    10800000000000::bigint,   -- 3h
                    21600000000000::bigint,   -- 6h
                    43200000000000::bigint,   -- 12h
                    86400000000000::bigint    -- 1d
                ],
                lambda w: span_ns // w <= target_buckets
            )),
            -- Past the ladder: whole days, rounded up so the count still fits.
            (span_ns / 86400000000000 / target_buckets + 1) * 86400000000000
        ) as bigint)
    end
)
