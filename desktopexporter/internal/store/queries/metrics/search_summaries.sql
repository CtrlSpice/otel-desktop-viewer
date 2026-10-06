{{.CTEs}},
		filtered_metrics as (
			select s.*
			{{.From}}
			where {{.Where}}
		),
		metric_latest_dp as (
			select d.metric_id, max(d.timestamp) as last_dp_ts
			from metric_datapoints d
			inner join filtered_metrics fm on d.metric_id = fm.id, search_params
			{{.DatapointWhere}}
			group by d.metric_id
		),
		candidate_metrics as (
			select fm.*
			from filtered_metrics fm
			left join metric_latest_dp mldp on mldp.metric_id = fm.id
			{{if .CandidateOrder}}order by {{.CandidateOrder}}{{.CandidateLimit}}{{end}}
		),
		filtered_dps as (
			select d.* from metric_datapoints d
			inner join candidate_metrics fm on d.metric_id = fm.id, search_params
			{{.DatapointWhere}}
		),
		-- series_count is window-scoped; series_cardinality covers all retained
		-- datapoints. Identity-only series rows do not count as active.
		metric_series_count as (
			select metric_id, count(distinct series_id) as series_count
			from filtered_dps
			group by metric_id
		),
		metric_series_cardinality as (
			select metric_id, count(distinct series_id) as series_cardinality
			from metric_datapoints
			where metric_id in (select id from candidate_metrics)
			group by metric_id
		),
		metric_datapoint_count as (
			select metric_id, count(*) as datapoint_count
			from filtered_dps
			group by metric_id
		),
		metric_last_value as (
			select
				d.metric_id,
				arg_max(coalesce(d.double_value, d.int_value), d.timestamp) as last_value
			from filtered_dps d
			inner join candidate_metrics fm on fm.id = d.metric_id
			where fm.metric_type in ('Gauge', 'Sum')
			group by d.metric_id
		),
		summary_rows as (
			select
				fm.id,
				fm.name,
				fm.description,
				fm.unit,
				fm.metric_type,
				fm.aggregation_temporality,
				fm.is_monotonic,
				fm.service_name,
				msc.series_count,
				coalesce(msc_all.series_cardinality, 0) as series_cardinality,
				mdc.datapoint_count,
				mlv.last_value,
				mldp.last_dp_ts
			from candidate_metrics fm
			left join metric_latest_dp mldp on mldp.metric_id = fm.id
			left join metric_series_count msc on msc.metric_id = fm.id
			left join metric_series_cardinality msc_all on msc_all.metric_id = fm.id
			left join metric_datapoint_count mdc on mdc.metric_id = fm.id
			left join metric_last_value mlv on mlv.metric_id = fm.id
		),
		selected_summaries as (
			select *
			from summary_rows
			order by {{.SummaryOrder}}{{.SummaryLimit}}
		)
		select cast(coalesce(to_json(list(json_object(
			'metricRef', cast(sub.id as varchar),
			'name', sub.name,
			'description', sub.description,
			'unit', sub.unit,
			'metricType', sub.metric_type,
			-- Code is received OTLP data; label is a display projection.
			'aggregationTemporalityCode', case
				when sub.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then sub.aggregation_temporality
				else null end,
			'aggregationTemporality', case
				when sub.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then case sub.aggregation_temporality
				when 0 then 'Unspecified'
				when 1 then 'Delta'
				when 2 then 'Cumulative'
				else 'Unknown (' || sub.aggregation_temporality::varchar || ')'
				end else null end,
			'isMonotonic', case
				when sub.metric_type = 'Sum' then sub.is_monotonic
				else null
			end,
			'serviceName', sub.service_name,
			'seriesCount', sub.series_count,
			'seriesCardinality', sub.series_cardinality,
			'dataPointCount', sub.datapoint_count,
			'lastValue', double_wire_json(sub.last_value),
			'lastSeen', sub.last_dp_ts::varchar
		) order by {{.SummaryOrder}})), '[]') as varchar) as summaries
		from selected_summaries sub
