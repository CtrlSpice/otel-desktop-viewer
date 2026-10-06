-- tz_offset_ns_at: the viewer's UTC offset at one instant, in nanoseconds.
--
-- Resolve the offset per timestamp to preserve DST boundaries. Integer
-- microseconds avoid floating-point rounding; ICU offsets are whole seconds.
-- A NULL zone returns NULL for caller-provided offset fallback.
create or replace macro tz_offset_ns_at(ts_ns, tz_name) as (
    case when tz_name is null then null
    else (
		epoch_us(make_timestamptz((ts_ns::hugeint // 1000)::bigint) AT TIME ZONE tz_name)::hugeint
		- ts_ns::hugeint // 1000
    ) * 1000
    end
)
