-- bucket_start_utc: the UTC instant a local-time bucket begins at.
--
-- ICU resolves ambiguous and skipped local times. Inputs are at least 1ms
-- apart, so conversion through microseconds is exact. Without a zone, subtract
-- the caller's fixed nanosecond offset.
create or replace macro bucket_start_utc(local_ns, tz_name, fallback_ns) as (
    case when tz_name is null then local_ns - fallback_ns
	else epoch_us(make_timestamp((local_ns::hugeint // 1000)::bigint) AT TIME ZONE tz_name)::hugeint * 1000
    end
)
