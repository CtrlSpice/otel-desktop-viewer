-- bucket_start_utc: the UTC instant a local-time bucket begins at.
--
-- Buckets are cut in local time, so that a "day" is the day the reader sees,
-- but they are joined and emitted as UTC instants. Going back is the harder
-- direction: the offset to subtract depends on the instant being solved for,
-- and near a transition the two sides of that fixed point disagree. ICU
-- resolves it properly -- a naive timestamp AT TIME ZONE is "this wall time
-- in this zone", including a deterministic answer for wall times a transition
-- made ambiguous or skipped -- so the conversion is delegated rather than
-- approximated. Zones that transition exactly at local midnight exist
-- (America/Santiago, America/Havana), so a day boundary can sit exactly on
-- the fixed point's bad case; approximating here mislabels those days.
--
-- Callers pass lattice points: multiples of a ladder rung, never finer than
-- 1ms, which is what makes the truncation to make_timestamp's microseconds
-- exact.
--
-- The spine and the bucketed rows both label buckets through this macro, and
-- they must agree byte-for-byte or their join drops readings -- one function,
-- called with the same arguments everywhere, is what guarantees that.
--
-- With no zone named there is nothing to resolve, and the caller's single
-- offset applies unchanged: the plain arithmetic zone-less callers keep.
create or replace macro bucket_start_utc(local_ns, tz_name, fallback_ns) as (
    case when tz_name is null then local_ns - fallback_ns
    else epoch_ns(make_timestamp(local_ns // 1000) AT TIME ZONE tz_name)
    end
)
