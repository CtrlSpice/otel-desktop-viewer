-- Lightweight exact Metric identity and series discovery. The series summary
-- fields are computed from retained datapoints; every other field is stored
-- received identity or a generated database-local ID.
with selected_metric as materialized (
	select * from metric_streams where id = ?::uuid
),
series_catalogue as materialized (
	select ms.id, ms.attribute_ids,
		count(d.id)::varchar as datapoint_count,
		min(d.timestamp)::varchar as first_datapoint_timestamp,
		max(d.timestamp)::varchar as last_datapoint_timestamp
	from metric_series ms
	join selected_metric m on m.id = ms.stream_id
	left join datapoints d on d.series_id = ms.id
	group by ms.id, ms.attribute_ids
)
select m.metric_type,
	cast(json_merge_patch(
		json_object(
			'metricID', m.id::varchar,
			'name', m.name,
			'unit', m.unit,
			'metricType', m.metric_type,
			'resource', json_object('attributes', attrs_json(m.resource_attribute_ids)),
			'scope', json_object(
				'name', m.scope_name,
				'version', m.scope_version,
				'attributes', attrs_json(m.scope_attribute_ids),
				'schemaUrl', m.scope_schema_url),
			'series', coalesce((
				select to_json(list(json_object(
					'seriesID', s.id::varchar,
					'attributes', attrs_json(s.attribute_ids),
					'datapointCount', s.datapoint_count,
					'firstDatapointTimestamp', s.first_datapoint_timestamp,
					'lastDatapointTimestamp', s.last_datapoint_timestamp)
					order by s.id))
				from series_catalogue s), json('[]'))),
		case m.metric_type
			when 'Sum' then json_object(
				'aggregationTemporalityCode', m.aggregation_temporality,
				'isMonotonic', m.is_monotonic)
			when 'Histogram' then json_object(
				'aggregationTemporalityCode', m.aggregation_temporality)
			when 'ExponentialHistogram' then json_object(
				'aggregationTemporalityCode', m.aggregation_temporality)
			else json('{}')
		end) as varchar) as document
from selected_metric m
