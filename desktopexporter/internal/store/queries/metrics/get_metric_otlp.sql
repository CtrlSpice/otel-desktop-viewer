-- Reconstruct exactly one received metric occurrence. The bound UUID names a
-- metric_ingests row, not a logical stream, series, metric name, or time range.
with selected as materialized (
	select mi.*, ms.name, ms.unit, ms.metric_type,
		ms.aggregation_temporality, ms.is_monotonic
	from metric_ingests mi
	join metric_streams ms on ms.id = mi.stream_id
	where mi.id = try_cast(? as uuid)
),
selected_datapoints as materialized (
	select d.*
	from datapoints d
	join selected s on s.id = d.metric_ingest_id
),
used_attribute_ids as materialized (
	select unnest(r.attribute_ids) as id
	from selected s join resources r on r.id = s.resource_id
	union
	select unnest(sc.attribute_ids)
	from selected s join scopes sc on sc.id = s.scope_id
	union
	select unnest(s.metadata_ids) from selected s
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
exemplar_documents as materialized (
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
datapoint_documents as materialized (
	select d.id, d.timestamp,
		case s.metric_type
			when 'Gauge' then json_merge_patch(
				json_object(
					'attributes', otlp_attributes(d.attribute_ids, (select values from converted_attributes)),
					'startTimeUnixNano', d.start_time::varchar,
					'timeUnixNano', d.timestamp::varchar,
					'exemplars', coalesce((select list(e.document order by e.timestamp, e.id) from exemplar_documents e where e.datapoint_id = d.id), []::json[]),
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
					'exemplars', coalesce((select list(e.document order by e.timestamp, e.id) from exemplar_documents e where e.datapoint_id = d.id), []::json[]),
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
					'exemplars', coalesce((select list(e.document order by e.timestamp, e.id) from exemplar_documents e where e.datapoint_id = d.id), []::json[]),
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
					'exemplars', coalesce((select list(e.document order by e.timestamp, e.id) from exemplar_documents e where e.datapoint_id = d.id), []::json[]),
					'flags', d.flags),
				case when d.sum is null then json('{}') else json_object('sum', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.sum)))) end,
				case when d.min is null then json('{}') else json_object('min', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.min)))) end,
				case when d.max is null then json('{}') else json_object('max', otlp_double_json(json_object('kind', 'double', 'value', double_wire_json(d.max)))) end)
			else null::json
		end as document
	from selected s
	join selected_datapoints d on true
	left join histogram_bounds hb on hb.id = d.bounds_id
),
metric_document as (
	select s.*,
		json_merge_patch(
			json_object(
				'name', s.name,
				'description', s.description,
				'unit', s.unit,
				'metadata', otlp_attributes(s.metadata_ids, (select values from converted_attributes))),
			case s.metric_type
				when 'Gauge' then json_object('gauge', json_object(
					'dataPoints', coalesce((select list(document order by timestamp, id) from datapoint_documents), []::json[])))
				when 'Sum' then json_object('sum', json_object(
					'dataPoints', coalesce((select list(document order by timestamp, id) from datapoint_documents), []::json[]),
					'aggregationTemporality', s.aggregation_temporality,
					'isMonotonic', s.is_monotonic))
				when 'Histogram' then json_object('histogram', json_object(
					'dataPoints', coalesce((select list(document order by timestamp, id) from datapoint_documents), []::json[]),
					'aggregationTemporality', s.aggregation_temporality))
				when 'ExponentialHistogram' then json_object('exponentialHistogram', json_object(
					'dataPoints', coalesce((select list(document order by timestamp, id) from datapoint_documents), []::json[]),
					'aggregationTemporality', s.aggregation_temporality))
				else null::json
			end) as document
	from selected s
)
select m.metric_type,
	case when m.document is null then null else otlp_document_text(json_object(
		'resourceMetrics', [json_object(
			'resource', otlp_resource(m.resource_id, (select values from converted_attributes)),
			'scopeMetrics', [json_object(
				'scope', otlp_scope(m.scope_id, (select values from converted_attributes)),
				'metrics', [m.document],
				'schemaUrl', m.scope_schema_url)],
			'schemaUrl', m.resource_schema_url)])) end as document
from metric_document m
