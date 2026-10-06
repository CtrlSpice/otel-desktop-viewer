with trace_spans as materialized (
	select *
	from spans
	where trace_id = ?::uuid
), bounds as (
	select min(start_time)::hugeint as trace_start,
		max(end_time)::hugeint as trace_end,
		count(*) as span_count
	from trace_spans
)
select cast(json_object(
	'trace', json_object(
		'traceID', trace_id_wire(any_value(s.trace_id)),
		'spanCount', b.span_count,
		'startTime', cast(b.trace_start as varchar),
		'durationNs', cast(b.trace_end - b.trace_start as varchar)
	),
	'spans', to_json(list(json_object(
		'spanID', span_id_wire(s.span_id),
		'parentSpanID', span_id_wire(s.parent_span_id),
		'service', s.service_name,
		'name', s.name,
		'startOffsetNs', cast(s.start_time::hugeint - b.trace_start as varchar),
		'durationNs', cast(s.end_time::hugeint - s.start_time::hugeint as varchar)
	) order by s.start_time asc, s.span_id asc))
) as varchar) as trace
from trace_spans s
cross join bounds b
group by b.trace_start, b.trace_end, b.span_count
