-- body_preview truncates a log body to what a summary card can show.
--
-- 200 characters: enough to recognise a message, short enough that a list of
-- them is not dominated by one stack trace. Callers needing the whole body
-- fetch the record through GetLog.
--
-- The length lives here rather than as a Go constant interpolated into the
-- query, which is what it was. That made the SQL non-static for a value that
-- never varies, and put a %d among the format's %s fragments where it read as
-- just another placeholder. As a macro the query text is fixed, the number sits
-- next to the truncation it describes, and tuning it is a one-line SQL edit
-- rather than a change that ripples through a params struct.
create or replace macro body_preview(body) as (
    substring(body, 1, 200)
)
