with search_params as (select ? as time_start, ? as time_end),
		trace_summaries as (
			select distinct on (s.trace_id)
				s.trace_id,
				(s.parent_span_id is null) as has_root_span,
				case when s.parent_span_id is null then nullif(s.service_name, '') end as service_name,
				case when s.parent_span_id is null then s.name end as root_name,
				min(s.start_time) over (partition by s.trace_id) as trace_start_time,
				max(s.end_time) over (partition by s.trace_id) as trace_end_time,
				count(*) over (partition by s.trace_id) as span_count,
				count(case when s.status_code = 'Error' then 1 end) over (partition by s.trace_id) as error_count
			from search_params, spans s
		join resources r on r.id = s.resource_id
		join scopes sc on sc.id = s.scope_id
			where (list_contains(s.attribute_ids, '9f1f8762-3dcf-10ee-4b04-db74f7de5b59'::uuid)) AND s.start_time >= time_start and s.start_time <= time_end
			order by
				s.trace_id,
				case when s.parent_span_id is null then 0 else 1 end
		),
		selected_summaries as (
			select *
			from trace_summaries
			order by trace_start_time desc, trace_id asc
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
					then (sub.trace_end_time - sub.trace_start_time)::varchar
				else null
			end,
			'spanCount',    sub.span_count,
			'errorCount',   sub.error_count
		) order by trace_start_time desc, trace_id asc
		)), '[]') as varchar) as summaries
		from selected_summaries sub
