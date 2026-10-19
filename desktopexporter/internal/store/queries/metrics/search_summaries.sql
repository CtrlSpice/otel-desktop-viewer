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
			select d.*, fm.metric_type from metric_datapoints d
			inner join candidate_metrics fm on d.metric_id = fm.id, search_params
			{{.DatapointWhere}}
		),
		-- series_count is window-scoped; series_cardinality covers all retained
		-- datapoints. Identity-only series rows do not count as active.
		-- Reuse the same per-series counts for totals and the selected breakdowns.
		series_counts as materialized (
			select metric_id, series_id, count(*) as datapoint_count
			from filtered_dps
			group by metric_id, series_id
		),
		metric_counts as (
			select metric_id, count(*) as series_count, sum(datapoint_count) as datapoint_count
			from series_counts
			group by metric_id
		),
		metric_series_cardinality as (
			select metric_id, count(distinct series_id) as series_cardinality
			from metric_datapoints
			where metric_id in (select id from candidate_metrics)
			group by metric_id
		),
		metric_last_value as (
			select
				d.metric_id,
				arg_max(coalesce(d.double_value, d.int_value), d.timestamp) as last_value
			from filtered_dps d
			where d.metric_type in ('Gauge', 'Sum')
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
				mc.series_count,
				coalesce(msc_all.series_cardinality, 0) as series_cardinality,
				mc.datapoint_count,
				mlv.last_value,
				mldp.last_dp_ts
			from candidate_metrics fm
			left join metric_latest_dp mldp on mldp.metric_id = fm.id
			left join metric_counts mc on mc.metric_id = fm.id
			left join metric_series_cardinality msc_all on msc_all.metric_id = fm.id
			left join metric_last_value mlv on mlv.metric_id = fm.id
		),
		selected_summaries as (
			select *
			from summary_rows
			order by {{.SummaryOrder}}{{.SummaryLimit}}
		),
		ranked_series as (
			select counts.*, row_number() over (
				partition by counts.metric_id order by counts.datapoint_count desc, counts.series_id
			) as rank
			from series_counts counts
			join selected_summaries selected on selected.id = counts.metric_id
		),
		series_parts as (
			select r.metric_id, r.series_id::varchar as key,
				coalesce((
					select string_agg(a.label, ', ' order by a.key, a.id)
					from (
						select a.id, a.key, substring(a.key, 1, 40) || '=' || body_preview_item(a.value) as label
						from unnest(s.attribute_ids) ids(id) join attributes a on a.id = ids.id
						order by a.key, a.id limit 3
					) a
				), 'No attributes') || case when len(s.attribute_ids) > 3 then ', …' else '' end as label,
				r.datapoint_count, false as other
			from ranked_series r join metric_series s on s.id = r.series_id
			where r.rank <= 8
			union all
			select metric_id, 'other-series', 'Other series', sum(datapoint_count), true
			from ranked_series where rank > 8 group by metric_id
		),
		series_breakdowns as (
			select metric_id, to_json(list(json_object(
				'key', key, 'label', label, 'count', datapoint_count, 'other', other
			) order by other, datapoint_count desc, key)) as parts
			from series_parts group by metric_id
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
			'seriesBreakdown', (select parts from series_breakdowns where metric_id = sub.id),
			'lastValue', double_wire_json(sub.last_value),
			'lastSeen', sub.last_dp_ts::varchar
		) order by {{.SummaryOrder}})), '[]') as varchar) as summaries
		from selected_summaries sub
