with matches as materialized (
	select *
	from spans
	where span_id = (select unnest(?::ubigint[]))
), selected as (
	select *
	from matches
	order by start_time desc, trace_id asc
	limit ?
)
select cast(coalesce(to_json(list(json_object(
	'traceID', trace_id_wire(trace_id),
	'spanID', span_id_wire(span_id),
	'parentSpanID', span_id_wire(parent_span_id),
	'service', service_name,
	'name', name,
	'startTime', start_time::varchar,
	'durationNs', (end_time::hugeint - start_time::hugeint)::varchar
) order by start_time desc, trace_id asc)), '[]') as varchar) as summaries,
	(select count(*) from matches) as match_count
from selected
