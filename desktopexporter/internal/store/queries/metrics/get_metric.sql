-- Lightweight exact Metric identity and series discovery. The series summary
-- fields are computed from retained datapoints; every other field is stored
-- received identity or a generated database-local ID.
with selected_metric as materialized (
	select m.*, r.schema_url as resource_schema_url,
		r.attribute_ids as resource_attribute_ids,
		r.dropped_attributes_count as resource_dropped_attributes_count,
		sc.name as scope_name, sc.version as scope_version,
		sc.attribute_ids as scope_attribute_ids, sc.schema_url as scope_schema_url,
		sc.dropped_attributes_count as scope_dropped_attributes_count
	from metric_streams m
	join resources r on r.id = m.resource_id
	join scopes sc on sc.id = m.scope_id
	where m.id = ?::uuid
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
			'metricRef', m.id::varchar,
			'name', m.name,
			'description', m.description,
			'unit', m.unit,
			'metadata', attrs_json(m.metadata_ids),
			'metricType', m.metric_type,
			'resource', json_object(
				'attributes', attrs_json(m.resource_attribute_ids),
				'droppedAttributesCount', m.resource_dropped_attributes_count,
				'schemaUrl', m.resource_schema_url),
			'scope', json_object(
				'name', m.scope_name,
				'version', m.scope_version,
				'attributes', attrs_json(m.scope_attribute_ids),
				'droppedAttributesCount', m.scope_dropped_attributes_count,
				'schemaUrl', m.scope_schema_url),
			'series', coalesce((
				select to_json(list(json_object(
					'seriesRef', s.id::varchar,
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
