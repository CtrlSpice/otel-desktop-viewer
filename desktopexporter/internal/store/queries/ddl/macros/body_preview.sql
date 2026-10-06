-- First 300 characters of a log body for summary cards.
create or replace macro body_preview(body) as (
	substring(coalesce(json_extract_string(body, '$.value'), body::varchar), 1, 300)
)
