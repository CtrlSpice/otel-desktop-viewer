-- bucket_width_ns: the time-bucket width to reduce a window to, in nanoseconds.
--
-- Choose the smallest fixed ladder rung producing at most target_buckets.
-- Absolute floor(timestamp / width) boundaries remain stable while panning.
-- NULL disables reduction.
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
