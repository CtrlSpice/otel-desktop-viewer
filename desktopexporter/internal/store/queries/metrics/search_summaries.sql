{{.CTEs}},
		filtered_ingests as (
			select m.id, m.stream_id
			{{.From}}
			where {{.Where}}
		),
		filtered_streams as (
			select s.* from metric_streams s
			where s.id in (select distinct stream_id from filtered_ingests)
		),
		filtered_dps as (
			select d.* from datapoints d
			inner join filtered_streams fs on d.stream_id = fs.id, search_params
			where d.timestamp >= time_start and d.timestamp <= time_end
		),
		stream_latest_dp as (
			select stream_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by stream_id
		),
		ingest_latest_dp as (
			select metric_ingest_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by metric_ingest_id
		),
		stream_description as (
			select mi.stream_id,
				arg_max(mi.description, ild.last_dp_ts) as description
			from metric_ingests mi
			inner join ingest_latest_dp ild on ild.metric_ingest_id = mi.id
			where mi.stream_id in (select id from filtered_streams)
			group by mi.stream_id
		),
		-- Counting series is now counting one indexable column, rather than
		-- distinct (resource, label-array) pairs.
		stream_series_count as (
			select stream_id, count(distinct series_id) as series_count
			from filtered_dps
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
			inner join filtered_streams fs on fs.id = d.stream_id
			where fs.metric_type in ('Gauge', 'Sum')
			group by d.stream_id
		)
		select cast(coalesce(to_json(list(json_object(
			'id', cast(fs.id as varchar),
			'name', fs.name,
			'description', sd.description,
			'unit', fs.unit,
			'metricType', fs.metric_type,
			'aggregationTemporality', fs.aggregation_temporality,
			'isMonotonic', case
				when fs.metric_type = 'Sum' then fs.is_monotonic
				else null
			end,
			'serviceName', fs.service_name,
			'seriesCount', ssc.series_count,
			'dataPointCount', sdc.datapoint_count,
			'lastValue', slv.last_value,
			'lastSeen', sldp.last_dp_ts::varchar
		) order by sldp.last_dp_ts desc nulls last)), '[]') as varchar) as summaries
		from filtered_streams fs
		left join stream_latest_dp sldp on sldp.stream_id = fs.id
		left join stream_description sd on sd.stream_id = fs.id
		left join stream_series_count ssc on ssc.stream_id = fs.id
		left join stream_datapoint_count sdc on sdc.stream_id = fs.id
		left join stream_last_value slv on slv.stream_id = fs.id
	