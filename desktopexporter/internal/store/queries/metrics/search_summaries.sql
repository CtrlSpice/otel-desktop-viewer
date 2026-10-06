{{.CTEs}},
		filtered_streams as (
			select s.*
			{{.From}}
			where {{.Where}}
		),
		stream_latest_dp as (
			select d.stream_id, max(d.timestamp) as last_dp_ts
			from datapoints d
			inner join filtered_streams fs on d.stream_id = fs.id, search_params
			{{.DatapointWhere}}
			group by d.stream_id
		),
		candidate_streams as (
			select fs.*
			from filtered_streams fs
			left join stream_latest_dp sldp on sldp.stream_id = fs.id
			{{if .CandidateOrder}}order by {{.CandidateOrder}}{{.CandidateLimit}}{{end}}
		),
		filtered_dps as (
			select d.* from datapoints d
			inner join candidate_streams fs on d.stream_id = fs.id, search_params
			{{.DatapointWhere}}
		),
		-- Two counts, because the card was showing one number that could mean
		-- either and said which only in a tooltip.
		--
		-- series_count is window-scoped: how many series actually reported in
		-- the range being looked at. It belongs with the numbers beside it --
		-- datapoint_count, last_value and the time range are all window-scoped
		-- too -- and it is the one that changes as you pan.
		--
		-- series_cardinality is the stream's retained-data total. Narrow the
		-- window on a race and the first drops to three while the second stays at
		-- twenty-one; neither is wrong, and showing only one makes the other look
		-- like a bug. Identity-only rows retained for stable generated ids do not
		-- count as active series.
		--
		-- Counting series is now counting one indexable column, rather than
		-- distinct (resource, label-array) pairs.
		stream_series_count as (
			select stream_id, count(distinct series_id) as series_count
			from filtered_dps
			group by stream_id
		),
		stream_series_cardinality as (
			select stream_id, count(distinct series_id) as series_cardinality
			from datapoints
			where stream_id in (select id from candidate_streams)
			group by stream_id
		),
		stream_datapoint_count as (
			select stream_id, count(*) as datapoint_count
			from filtered_dps
			group by stream_id
		),
		stream_last_value as (
			select
				d.stream_id,
				arg_max(coalesce(d.double_value, d.int_value), d.timestamp) as last_value
			from filtered_dps d
			inner join candidate_streams fs on fs.id = d.stream_id
			where fs.metric_type in ('Gauge', 'Sum')
			group by d.stream_id
		),
		summary_rows as (
			select
				fs.id,
				fs.name,
				fs.description,
				fs.unit,
				fs.metric_type,
				fs.aggregation_temporality,
				fs.is_monotonic,
				fs.service_name,
				ssc.series_count,
				coalesce(ssx.series_cardinality, 0) as series_cardinality,
				sdc.datapoint_count,
				slv.last_value,
				sldp.last_dp_ts
			from candidate_streams fs
			left join stream_latest_dp sldp on sldp.stream_id = fs.id
			left join stream_series_count ssc on ssc.stream_id = fs.id
			left join stream_series_cardinality ssx on ssx.stream_id = fs.id
			left join stream_datapoint_count sdc on sdc.stream_id = fs.id
			left join stream_last_value slv on slv.stream_id = fs.id
		),
		selected_summaries as (
			select *
			from summary_rows
			order by {{.SummaryOrder}}{{.SummaryLimit}}
		)
		select cast(coalesce(to_json(list(json_object(
			'id', cast(sub.id as varchar),
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
