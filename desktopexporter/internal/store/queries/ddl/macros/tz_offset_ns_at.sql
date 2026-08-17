-- tz_offset_ns_at: the viewer's UTC offset at one instant, in nanoseconds.
--
-- Bucket boundaries are shifted by this so a day breaks where the reader's
-- calendar says it does. A single offset, captured once and applied to every
-- timestamp, is right until the window crosses a DST transition or the data is
-- from the other side of the year: a viewer in Europe/London looking at July
-- data in December gets GMT, and every day column lands an hour off local
-- midnight -- with a datapoint at 00:30 BST filed under the previous day.
--
-- Resolved per timestamp, so each bucket uses the offset in force at its own
-- moment. The zone itself is ICU's to answer -- AT TIME ZONE turns the instant
-- into the zone's wall time -- and the difference is taken in integer
-- microseconds, where "instant minus instant" cannot pick up a rounding ulp
-- the way a detour through doubles can. Truncating to microseconds is exact
-- for the subtraction because both sides truncate the same value, and offsets
-- themselves are whole seconds in every zone ICU knows.
--
-- Null zone yields null, which lets the caller fall back to the offset it
-- sent: a client that names no zone keeps the old behaviour rather than
-- silently switching to UTC.
create or replace macro tz_offset_ns_at(ts_ns, tz_name) as (
    case when tz_name is null then null
    else (
        epoch_us(make_timestamptz(ts_ns // 1000) AT TIME ZONE tz_name)
        - ts_ns // 1000
    ) * 1000
    end
)
