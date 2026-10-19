{{.CTEs}},
		eligible_spans as materialized (
			select distinct s.trace_id
			{{.From}}
			where {{.EligibilityWhere}}
		),
		selected_trace_ids as materialized (
			select trace_id from eligible_spans
		),{{.MatchCTEs}}
		trace_summaries as (
			select distinct on (s.trace_id)
				s.trace_id,
				(s.parent_span_id is null) as has_root_span,
				case when s.parent_span_id is null then nullif(s.service_name, '') end as service_name,
				case when s.parent_span_id is null then s.name end as root_name,
				min(s.start_time) over (partition by s.trace_id) as trace_start_time,
				max(s.end_time) over (partition by s.trace_id) as trace_end_time,
				count(*) over (partition by s.trace_id) as span_count,
				count(case when s.status_code = 2 then 1 end) over (partition by s.trace_id) as error_count
			from spans s
			join selected_trace_ids selected on selected.trace_id = s.trace_id
			order by
				s.trace_id,
				case when s.parent_span_id is null then 0 else 1 end
		),
		selected_summaries as (
			select *
			from trace_summaries
			order by {{.Order}}{{.Limit}}
		),
		service_counts as (
			select s.trace_id, coalesce(s.service_name, '') as service_name, count(*) as span_count
			from spans s
			join selected_summaries selected on selected.trace_id = s.trace_id
			group by s.trace_id, coalesce(s.service_name, '')
		),
		ranked_services as (
			select *, row_number() over (partition by trace_id order by span_count desc, service_name) as rank
			from service_counts
		),
		service_parts as (
			select trace_id, 'service:' || service_name as key,
				coalesce(nullif(service_name, ''), 'Unknown service') as label,
				span_count, false as other
			from ranked_services where rank <= 8
			union all
			select trace_id, 'other-services', 'Other services', sum(span_count), true
			from ranked_services where rank > 8 group by trace_id
		),
		service_breakdowns as (
			select trace_id, to_json(list(json_object(
				'key', key, 'label', label, 'count', span_count, 'other', other
			) order by other, span_count desc, key)) as parts
			from service_parts group by trace_id
		)
		select cast(coalesce(to_json(list(json_object(
			'traceID',      replace(sub.trace_id::varchar, '-', ''),
			'hasRootSpan',  sub.has_root_span,
			'rootSpan',     case when sub.has_root_span then json_object(
				'serviceName', sub.service_name,
				'name',        sub.root_name
			) end,
			'startTime',    sub.trace_start_time::varchar,
			'durationNs',   case
				when sub.trace_start_time is not null
					and sub.trace_end_time is not null
					then (sub.trace_end_time::hugeint - sub.trace_start_time::hugeint)::varchar
				else null
			end,
			'spanCount',    sub.span_count,
			'errorCount',   sub.error_count,
			'serviceBreakdown', (select parts from service_breakdowns where trace_id = sub.trace_id){{.MatchProjection}}
		) order by {{.Order}}
		)), '[]') as varchar) as summaries
		from selected_summaries sub{{.MatchJoin}}
