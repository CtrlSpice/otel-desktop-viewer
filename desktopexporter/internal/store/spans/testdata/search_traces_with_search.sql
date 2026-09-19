with search_params as (select unnest(?::ubigint[]) as time_start, unnest(?::ubigint[]) as time_end, ? as attr_key_2, ? as attr_kind_3, ? as value_4),
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
			where (exists(
			select 1 from unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid
			where a.key = attr_key_2 and json_extract_string(a.value, '$.kind') = attr_kind_3 and coalesce(json_extract_string(a.value, '$.value'), json_extract(a.value, '$.value')::varchar) = value_4
		)) AND s.start_time >= time_start and s.start_time <= time_end
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
					then (sub.trace_end_time::hugeint - sub.trace_start_time::hugeint)::varchar
				else null
			end,
			'spanCount',    sub.span_count,
			'errorCount',   sub.error_count
		) order by trace_start_time desc, trace_id asc
		)), '[]') as varchar) as summaries
		from selected_summaries sub
