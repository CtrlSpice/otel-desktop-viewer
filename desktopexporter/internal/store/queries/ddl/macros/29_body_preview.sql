-- body_preview truncates a log body for the summary card. Callers needing the
-- whole body fetch the record through GetLog.
--
-- The length lives here rather than as a Go constant interpolated into the
-- query, which is what it was: that made the query text non-static for a value
-- that never varies.
create or replace macro body_preview(body) as (
    substring(body, 1, 300)
)
