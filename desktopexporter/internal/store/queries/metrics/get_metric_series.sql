-- Exact retained received data for one generated database-local Metric/series
-- pair. Bounds apply only to received datapoint timestamps.
with selected_metric as materialized (
	select m.* from metrics m
	where m.id = $1::uuid
),
selected_series as materialized (
	select s.* from metric_series s
	where s.id = $2::uuid and s.metric_id = $1::uuid
),
selected_datapoints as materialized (
	select d.*
	from metric_datapoints d
	join selected_series s on s.id = d.series_id
	where {{if .ContextOnly}}false and {{end}}(list_extract($3::ubigint[], 1) is null or d.timestamp >= list_extract($3::ubigint[], 1))
	  and (list_extract($4::ubigint[], 1) is null or d.timestamp <= list_extract($4::ubigint[], 1))
),
selected_exemplars as materialized (
	select e.* from exemplars e
	join selected_datapoints d on d.id = e.metric_datapoint_id
){{if arrow}}, exemplar_documents as (
    select e.metric_datapoint_id, list({{template "exemplar"}} order by e.timestamp, e.id) as documents
    from selected_exemplars e group by e.metric_datapoint_id
)
select {{template "metricColumns"}}, s.id::varchar as seriesRef, s.attributes,
    (select coalesce(list({{template "receivedPoint"}} order by d.timestamp, d.id), [])
     from selected_datapoints d left join exemplar_documents e on e.metric_datapoint_id = d.id) as datapoints
from selected_metric m join selected_series s on s.metric_id = m.id
{{else}}, exemplar_documents as materialized (
	select e.metric_datapoint_id,
		to_json(list(exemplar_json(e, attribute_document(e.attributes)) order by e.timestamp, e.id)) as documents
	from selected_exemplars e
	group by e.metric_datapoint_id
),
datapoint_documents as materialized (
	select d.id, d.timestamp,
		case m.metric_type
			when 'Gauge' then json_object(
					'datapointRef', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'valueType', d.value_type,
					'doubleValue', double_wire_json(d.double_value),
					'intValue', d.int_value::varchar)
			when 'Sum' then json_object(
					'datapointRef', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'valueType', d.value_type,
					'doubleValue', double_wire_json(d.double_value),
					'intValue', d.int_value::varchar)
			when 'Histogram' then json_merge_patch(
				json_object(
					'datapointRef', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'count', d.count::varchar,
					'bucketCounts', list_transform(d.bucket_counts, lambda n: n::varchar),
					'explicitBounds', list_transform(d.explicit_bounds, lambda bound: double_wire_json(bound))),
				case when d.sum is null then json('{}') else json_object('sum', double_wire_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', double_wire_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', double_wire_json(d.max)) end)
			when 'ExponentialHistogram' then json_merge_patch(
				json_object(
					'datapointRef', d.id::varchar,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'exemplars', coalesce(e.documents, json('[]')),
					'count', d.count::varchar,
					'scale', d.scale,
					'zeroCount', d.zero_count::varchar,
					'zeroThreshold', double_wire_json(d.zero_threshold),
					'positive', json_object('offset', d.positive_bucket_offset,
						'bucketCounts', list_transform(d.positive_bucket_counts, lambda n: n::varchar)),
					'negative', json_object('offset', d.negative_bucket_offset,
						'bucketCounts', list_transform(d.negative_bucket_counts, lambda n: n::varchar))),
				case when d.sum is null then json('{}') else json_object('sum', double_wire_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', double_wire_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', double_wire_json(d.max)) end)
			else null::json
		end as document
	from selected_datapoints d
	cross join selected_metric m
	left join exemplar_documents e on e.metric_datapoint_id = d.id
)
select m.metric_type,
	cast(json_merge_patch(
		json_object(
			'metricRef', m.id::varchar,
			'name', m.name,
			'description', m.description,
			'unit', m.unit,
			'metadata', attribute_document(m.metadata),
			'metricType', m.metric_type,
			'resource', json_object(
				'attributes', attribute_document(m.resource.attributes),
				'droppedAttributesCount', m.resource.dropped_attributes_count,
				'schemaUrl', m.resource.schema_url),
			'scope', json_object(
				'name', m.scope.name,
				'version', m.scope.version,
				'attributes', attribute_document(m.scope.attributes),
				'droppedAttributesCount', m.scope.dropped_attributes_count,
				'schemaUrl', m.scope.schema_url),
			'seriesRef', s.id::varchar,
			'attributes', attribute_document(s.attributes),
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
join selected_series s on s.metric_id = m.id{{end}}
