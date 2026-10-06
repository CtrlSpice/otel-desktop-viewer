-- Exact retained received data for one generated database-local Metric/series
-- pair. Bounds apply only to received datapoint timestamps.
with input as (
	select ?::uuid as metric_id, ?::uuid as series_id,
		list_extract(?::ubigint[], 1) as time_start,
		list_extract(?::ubigint[], 1) as time_end
),
selected_metric as materialized (
	select m.*, r.attribute_ids as resource_attribute_ids,
		r.dropped_attributes_count as resource_dropped_attributes_count,
		sc.name as scope_name, sc.version as scope_version,
		sc.attribute_ids as scope_attribute_ids, sc.schema_url as scope_schema_url,
		sc.dropped_attributes_count as scope_dropped_attributes_count
	from metric_streams m
	join resources r on r.id = m.resource_id
	join scopes sc on sc.id = m.scope_id, input i
	where m.id = i.metric_id
),
selected_series as materialized (
	select s.* from metric_series s, input i
	where s.id = i.series_id and s.stream_id = i.metric_id
),
selected_datapoints as materialized (
	select d.*, hb.bounds as explicit_bounds
	from datapoints d
	join selected_series s on s.id = d.series_id
	left join histogram_bounds hb on hb.id = d.bounds_id,
		input i
	where (i.time_start is null or d.timestamp >= i.time_start)
	  and (i.time_end is null or d.timestamp <= i.time_end)
),
exemplar_documents as materialized (
	select e.datapoint_id,
		to_json(list(exemplar_json(e) order by e.timestamp, e.id)) as documents
	from exemplars e
	join selected_datapoints d on d.id = e.datapoint_id
	group by e.datapoint_id
),
datapoint_documents as materialized (
	select d.id, d.timestamp,
		case m.metric_type
			when 'Gauge' then json_object(
					'datapointID', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'valueType', d.value_type,
					'doubleValue', double_wire_json(d.double_value),
					'intValue', d.int_value::varchar)
			when 'Sum' then json_object(
					'datapointID', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'valueType', d.value_type,
					'doubleValue', double_wire_json(d.double_value),
					'intValue', d.int_value::varchar)
			when 'Histogram' then json_merge_patch(
				json_object(
					'datapointID', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'count', d.count::varchar,
					'bucketCounts', list_transform(d.bucket_counts, n -> n::varchar),
					'explicitBounds', list_transform(d.explicit_bounds, bound -> double_wire_json(bound))),
				case when d.sum is null then json('{}') else json_object('sum', double_wire_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', double_wire_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', double_wire_json(d.max)) end)
			when 'ExponentialHistogram' then json_merge_patch(
				json_object(
					'datapointID', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'count', d.count::varchar,
					'scale', d.scale,
					'zeroCount', d.zero_count::varchar,
					'zeroThreshold', double_wire_json(d.zero_threshold),
					'positive', json_object('offset', d.positive_bucket_offset,
						'bucketCounts', list_transform(d.positive_bucket_counts, n -> n::varchar)),
					'negative', json_object('offset', d.negative_bucket_offset,
						'bucketCounts', list_transform(d.negative_bucket_counts, n -> n::varchar))),
				case when d.sum is null then json('{}') else json_object('sum', double_wire_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', double_wire_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', double_wire_json(d.max)) end)
			else null::json
		end as document
	from selected_datapoints d
	cross join selected_metric m
	left join exemplar_documents e on e.datapoint_id = d.id
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
			'seriesRef', s.id::varchar,
			'attributes', attrs_json(s.attribute_ids),
			'datapoints', coalesce((select to_json(list(d.document order by d.timestamp, d.id)) from datapoint_documents d), json('[]'))),
		case m.metric_type
			when 'Sum' then json_object(
				'aggregationTemporalityCode', m.aggregation_temporality,
				'isMonotonic', m.is_monotonic)
			when 'Histogram' then json_object('aggregationTemporalityCode', m.aggregation_temporality)
			when 'ExponentialHistogram' then json_object('aggregationTemporalityCode', m.aggregation_temporality)
			else json('{}')
		end) as varchar) as document
from selected_metric m
join selected_series s on s.stream_id = m.id
