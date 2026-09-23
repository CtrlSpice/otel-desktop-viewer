-- Reconstruct all retained reports for one metric stream. The bound UUID names
-- a metric_streams row; metric_ingests IDs only associate stored datapoints.
with selected_stream as materialized (
	select * from metric_streams where id = try_cast(? as uuid)
),
selected_ingests as materialized (
	select mi.*
	from metric_ingests mi
	join selected_stream s on s.id = mi.stream_id
),
selected_datapoints as materialized (
	select d.*
	from datapoints d
	join selected_stream s on s.id = d.stream_id
	join selected_ingests i on i.id = d.metric_ingest_id
),
used_attribute_ids as materialized (
	select unnest(r.attribute_ids) as id
	from (select distinct resource_id from selected_ingests) owners
	join resources r on r.id = owners.resource_id
	union
	select unnest(sc.attribute_ids)
	from (select distinct scope_id from selected_ingests) owners
	join scopes sc on sc.id = owners.scope_id
	union
	select unnest(i.metadata_ids) from selected_ingests i
	union
	select unnest(d.attribute_ids) from selected_datapoints d
	union
	select unnest(e.attribute_ids)
	from exemplars e join selected_datapoints d on d.id = e.datapoint_id
),
attribute_batch_input as materialized (
	select
		coalesce(list(used.id order by used.id), []::uuid[]) as ids,
		coalesce(list(a.value order by used.id), []::json[]) as encoded_values
	from used_attribute_ids used
	left join attributes a using (id)
),
attribute_batch as materialized (
	select input.ids, list(converted.value order by converted.source_id) as values
	from attribute_batch_input input
	cross join otlp_any_values((select encoded_values from attribute_batch_input)) converted
	group by input.ids
),
converted_attributes as materialized (
	select coalesce((
		select list(struct_pack(
			id := batch.ids[position],
			value := batch.values[position]) order by position)
		from attribute_batch batch
		cross join range(1, len(batch.ids) + 1) positions(position)
	), []::struct(id uuid, value json)[]) as values
),
exemplar_documents_unaggregated as materialized (
	select e.datapoint_id, e.timestamp, e.id,
		json_merge_patch(
			json_object(
				'filteredAttributes', otlp_attributes(e.attribute_ids, (select values from converted_attributes)),
				'timeUnixNano', e.timestamp::varchar),
			case
				when e.int_value is not null then json_object('asInt', e.int_value::varchar)
				when e.double_value is not null then json_object('asDouble', otlp_double_json(
					json_object('kind', 'double', 'value', double_wire_json(e.double_value))))
				else json('{}')
			end,
			case when e.trace_id is null then json('{}') else json_object('traceId', trace_id_wire(e.trace_id)) end,
			case when e.span_id is null then json('{}') else json_object('spanId', span_id_wire(e.span_id)) end
		) as document
	from exemplars e
	join selected_datapoints d on d.id = e.datapoint_id
),
exemplar_documents as materialized (
	select datapoint_id, list(document order by timestamp, id) as documents
	from exemplar_documents_unaggregated
	group by datapoint_id
),
datapoint_documents as materialized (
	select d.id, d.metric_ingest_id, d.timestamp,
		case s.metric_type
			when 'Gauge' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attribute_ids, (select values from converted_attributes)),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case d.value_type
					when 'Int' then case when d.int_value is null then error('stored metric int oneof has no value')::json else json_object('asInt', d.int_value::varchar) end
					when 'Double' then case when d.double_value is null then error('stored metric double oneof has no value')::json else json_object('asDouble', otlp_double_json(
						json_object('kind', 'double', 'value', double_wire_json(d.double_value)))) end
					when 'Empty' then json('{}')
					else error('unknown stored metric number value type: ' || coalesce(d.value_type, 'SQL NULL'))::json
				end)
			when 'Sum' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attribute_ids, (select values from converted_attributes)),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case d.value_type
					when 'Int' then case when d.int_value is null then error('stored metric int oneof has no value')::json else json_object('asInt', d.int_value::varchar) end
					when 'Double' then case when d.double_value is null then error('stored metric double oneof has no value')::json else json_object('asDouble', otlp_double_json(
						json_object('kind', 'double', 'value', double_wire_json(d.double_value)))) end
					when 'Empty' then json('{}')
					else error('unknown stored metric number value type: ' || coalesce(d.value_type, 'SQL NULL'))::json
				end)
			when 'Histogram' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attribute_ids, (select values from converted_attributes)),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'count', d.count::varchar,
					'bucketCounts', list_transform(d.bucket_counts, count -> count::varchar),
					'explicitBounds', list_transform(hb.bounds, bound -> otlp_double_json(
						json_object('kind', 'double', 'value', double_wire_json(bound)))),
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case when d.sum is null then json('{}') else json_object('sum', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.sum)))) end,
				case when d.min is null then json('{}') else json_object('min', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.min)))) end,
				case when d.max is null then json('{}') else json_object('max', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.max)))) end)
			when 'ExponentialHistogram' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attribute_ids, (select values from converted_attributes)),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'count', d.count::varchar,
					'scale', d.scale,
					'zeroCount', d.zero_count::varchar,
					'positive', json_object('offset', d.positive_bucket_offset, 'bucketCounts', list_transform(d.positive_bucket_counts, count -> count::varchar)),
					'negative', json_object('offset', d.negative_bucket_offset, 'bucketCounts', list_transform(d.negative_bucket_counts, count -> count::varchar)),
					'zeroThreshold', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.zero_threshold))),
					'exemplars', coalesce(e.documents, []::json[]),
					'flags', d.flags),
				case when d.sum is null then json('{}') else json_object('sum', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.sum)))) end,
				case when d.min is null then json('{}') else json_object('min', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.min)))) end,
				case when d.max is null then json('{}') else json_object('max', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.max)))) end)
			else null::json
		end as document
	from selected_datapoints d
	cross join selected_stream s
	left join histogram_bounds hb on hb.id = d.bounds_id
	left join exemplar_documents e on e.datapoint_id = d.id
),
grouped_metrics as materialized (
	select i.resource_id, i.resource_schema_url, i.scope_id, i.scope_schema_url,
		i.description, i.metadata_ids, s.name, s.unit, s.metric_type,
		s.aggregation_temporality, s.is_monotonic,
		coalesce(list(d.document order by d.timestamp, d.id) filter (where d.id is not null), []::json[]) as datapoints
	from selected_ingests i
	cross join selected_stream s
	left join datapoint_documents d on d.metric_ingest_id = i.id
	group by i.resource_id, i.resource_schema_url, i.scope_id, i.scope_schema_url,
		i.description, i.metadata_ids, s.name, s.unit, s.metric_type,
		s.aggregation_temporality, s.is_monotonic
),
metric_documents as materialized (
	select g.*,
		json_merge_patch(
			json_object('name', g.name, 'description', g.description, 'unit', g.unit,
				'metadata', otlp_attributes(g.metadata_ids, (select values from converted_attributes))),
			case g.metric_type
				when 'Gauge' then json_object('gauge', json_object('dataPoints', g.datapoints))
				when 'Sum' then json_object('sum', json_object('dataPoints', g.datapoints,
					'aggregationTemporality', g.aggregation_temporality, 'isMonotonic', g.is_monotonic))
				when 'Histogram' then json_object('histogram', json_object('dataPoints', g.datapoints,
					'aggregationTemporality', g.aggregation_temporality))
				when 'ExponentialHistogram' then json_object('exponentialHistogram', json_object('dataPoints', g.datapoints,
					'aggregationTemporality', g.aggregation_temporality))
				else null::json
			end) as document
	from grouped_metrics g
),
scope_documents as materialized (
	select resource_id, resource_schema_url, scope_id, scope_schema_url,
		json_object(
			'scope', otlp_scope(scope_id, (select values from converted_attributes)),
			'metrics', list(document order by description, metadata_ids),
			'schemaUrl', scope_schema_url) as document
	from metric_documents
	group by resource_id, resource_schema_url, scope_id, scope_schema_url
),
resource_documents as materialized (
	select resource_id, resource_schema_url,
		json_object(
			'resource', otlp_resource(resource_id, (select values from converted_attributes)),
			'scopeMetrics', list(document order by scope_id, scope_schema_url),
			'schemaUrl', resource_schema_url) as document
	from scope_documents
	group by resource_id, resource_schema_url
)
select s.metric_type,
	case
		when s.metric_type not in ('Gauge', 'Sum', 'Histogram', 'ExponentialHistogram') then null
		else otlp_document_text(json_object(
			'resourceMetrics', coalesce(list(r.document order by r.resource_id, r.resource_schema_url)
				filter (where r.document is not null), []::json[])))
	end as document
from selected_stream s
left join resource_documents r on true
group by s.metric_type
