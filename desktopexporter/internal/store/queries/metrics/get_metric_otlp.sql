-- Reconstruct one retained Metric and its ResourceMetrics/ScopeMetrics wrappers.
with export_selection as (
	select try_cast(? as uuid) as series_id, try_cast(? as uuid) as datapoint_id,
		?::boolean as include_datapoints
),
selected_metric as materialized (
	select * from metrics where id = try_cast(? as uuid)
),
matching_datapoints as not materialized (
	select d.*
	from metric_datapoints d
	join selected_metric s on s.id = d.metric_id
	cross join export_selection e
	where (e.series_id is null or d.series_id = e.series_id)
	  and (e.datapoint_id is null or d.id = e.datapoint_id)
),
selected_datapoints as materialized (
	select * from matching_datapoints
	where (select include_datapoints from export_selection)
),
exemplar_documents_unaggregated as materialized (
	select e.metric_datapoint_id, e.timestamp, e.id,
		json_merge_patch(
			json_object(
				'filteredAttributes', otlp_attributes(e.attributes),
				'timeUnixNano', e.timestamp::varchar),
			case
				when e.int_value is not null then json_object('asInt', e.int_value::varchar)
				when e.double_value is not null then json_object('asDouble', otlp_double_json(e.double_value))
				else json('{}')
			end,
			case when e.trace_id is null then json('{}') else json_object('traceId', trace_id_wire(e.trace_id)) end,
			case when e.span_id is null then json('{}') else json_object('spanId', span_id_wire(e.span_id)) end
		) as document
	from exemplars e
	join selected_datapoints d on d.id = e.metric_datapoint_id
),
exemplar_documents as materialized (
	select metric_datapoint_id, list(document order by timestamp, id) as documents
	from exemplar_documents_unaggregated
	group by metric_datapoint_id
),
datapoint_documents as materialized (
	select d.id, d.timestamp, d.resource, d.scope,
		case s.metric_type
			when 'Gauge' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attributes),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case d.value_type
					when 'Int' then case when d.int_value is null then error('stored metric int oneof has no value')::json else json_object('asInt', d.int_value::varchar) end
					when 'Double' then case when d.double_value is null then error('stored metric double oneof has no value')::json else json_object('asDouble', otlp_double_json(d.double_value)) end
					when 'Empty' then json('{}')
					else error('unknown stored metric number value type: ' || coalesce(d.value_type, 'SQL NULL'))::json
				end)
			when 'Sum' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attributes),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case d.value_type
					when 'Int' then case when d.int_value is null then error('stored metric int oneof has no value')::json else json_object('asInt', d.int_value::varchar) end
					when 'Double' then case when d.double_value is null then error('stored metric double oneof has no value')::json else json_object('asDouble', otlp_double_json(d.double_value)) end
					when 'Empty' then json('{}')
					else error('unknown stored metric number value type: ' || coalesce(d.value_type, 'SQL NULL'))::json
				end)
			when 'Histogram' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attributes),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'count', d.count::varchar,
					'bucketCounts', list_transform(d.bucket_counts, lambda count: count::varchar),
					'explicitBounds', list_transform(hb.bounds, lambda bound: otlp_double_json(bound)),
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case when d.sum is null then json('{}') else json_object('sum', otlp_double_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', otlp_double_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', otlp_double_json(d.max)) end)
			when 'ExponentialHistogram' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attributes),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'count', d.count::varchar,
					'scale', d.scale,
					'zeroCount', d.zero_count::varchar,
					'positive', json_object('offset', d.positive_bucket_offset, 'bucketCounts', list_transform(d.positive_bucket_counts, lambda count: count::varchar)),
					'negative', json_object('offset', d.negative_bucket_offset, 'bucketCounts', list_transform(d.negative_bucket_counts, lambda count: count::varchar)),
					'zeroThreshold', otlp_double_json(d.zero_threshold),
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case when d.sum is null then json('{}') else json_object('sum', otlp_double_json(d.sum)) end,
				case when d.min is null then json('{}') else json_object('min', otlp_double_json(d.min)) end,
				case when d.max is null then json('{}') else json_object('max', otlp_double_json(d.max)) end)
			else null::json
		end as document
	from selected_datapoints d
	cross join selected_metric s
	left join histogram_bounds hb on hb.id = d.bounds_id
	left join exemplar_documents e on e.metric_datapoint_id = d.id
),
grouped_metrics as materialized (
	select coalesce(d.resource, s.resource).id as resource_id,
		coalesce(d.resource, s.resource).schema_url as resource_schema_url,
		coalesce(d.scope, s.scope).id as scope_id,
		coalesce(d.scope, s.scope).schema_url as scope_schema_url,
		coalesce(d.resource, s.resource) as resource, coalesce(d.scope, s.scope) as scope,
		s.description, s.metadata, s.name, s.unit, s.metric_type,
		s.aggregation_temporality, s.is_monotonic,
		case when (select include_datapoints from export_selection)
			then json_object('dataPoints', coalesce(list(d.document order by d.timestamp, d.id)
				filter (where d.id is not null), []::json[]))
			else json('{}') end as datapoints_document
	from selected_metric s
	left join datapoint_documents d on true
	group by coalesce(d.resource, s.resource), coalesce(d.scope, s.scope),
		s.description, s.metadata, s.name, s.unit, s.metric_type,
		s.aggregation_temporality, s.is_monotonic
),
metric_documents as materialized (
	select g.*,
		json_merge_patch(
			json_object('name', g.name, 'description', g.description, 'unit', g.unit,
				'metadata', otlp_attributes(g.metadata)),
			case g.metric_type
				when 'Gauge' then json_object('gauge', g.datapoints_document)
				when 'Sum' then json_object('sum', json_merge_patch(g.datapoints_document,
					json_object('aggregationTemporality', g.aggregation_temporality, 'isMonotonic', g.is_monotonic)))
				when 'Histogram' then json_object('histogram', json_merge_patch(g.datapoints_document,
					json_object('aggregationTemporality', g.aggregation_temporality)))
				when 'ExponentialHistogram' then json_object('exponentialHistogram', json_merge_patch(g.datapoints_document,
					json_object('aggregationTemporality', g.aggregation_temporality)))
				else null::json
			end) as document
	from grouped_metrics g
),
scope_documents as materialized (
	select resource_id, resource_schema_url, resource, scope_id, scope_schema_url,
		json_object(
			'scope', otlp_scope(scope),
			'metrics', list(document order by description, metadata),
			'schemaUrl', scope_schema_url) as document
	from metric_documents
	group by resource_id, resource_schema_url, resource, scope_id, scope_schema_url, scope
),
resource_documents as materialized (
	select resource_id, resource_schema_url,
		json_object(
			'resource', otlp_resource(resource),
			'scopeMetrics', list(document order by scope_id, scope_schema_url),
			'schemaUrl', resource_schema_url) as document
	from scope_documents
	group by resource_id, resource_schema_url, resource
)
select s.metric_type,
	case
		when s.metric_type not in ('Gauge', 'Sum', 'Histogram', 'ExponentialHistogram') then null
		else otlp_document_text(json_object(
			'resourceMetrics', coalesce(list(r.document order by r.resource_id, r.resource_schema_url)
				filter (where r.document is not null), []::json[])))
	end as document
from selected_metric s
left join resource_documents r on true
where ((select series_id from export_selection) is null and (select datapoint_id from export_selection) is null)
   or exists (select 1 from matching_datapoints)
group by s.metric_type
