-- Reconstruct the one stored log record identified by the bound tool-minted
-- UUID. Received and observed timestamps remain separate.
with selected as (
	select l.*
	from logs l
	where l.id = try_cast(? as uuid)
),
used_attribute_ids as materialized (
	select unnest(attribute_ids) as id from selected
	union
	select unnest(r.attribute_ids) from resources r
	where r.id in (select resource_id from selected)
	union
	select unnest(sc.attribute_ids) from scopes sc
	where sc.id in (select scope_id from selected)
),
attribute_batch_input as materialized (
	select
		coalesce(list(used.id order by used.id), []::uuid[]) as ids,
		coalesce(list(a.value order by used.id), []::json[]) as encoded_values
	from used_attribute_ids used
	left join attributes a using (id)
),
value_batch_input as materialized (
	select
		(select id from selected) as id,
		case when exists (select 1 from selected)
			then list_prepend((select body from selected), input.encoded_values)
			else []::json[]
		end as encoded_values
	from attribute_batch_input input
),
value_batch as materialized (
	select input.id, list(converted.value order by converted.source_id) as values
	from value_batch_input input
	cross join otlp_any_values((select encoded_values from value_batch_input)) converted
	group by input.id
),
converted_attributes as materialized (
	select list(struct_pack(
		id := input.ids[position],
		value := batch.values[position + 1]) order by position) as values
	from attribute_batch_input input
	cross join value_batch batch
	cross join range(1, len(input.ids) + 1) positions(position)
),
log_documents as (
	select
		l.resource_id,
		l.resource_schema_url,
		l.scope_id,
		l.scope_schema_url,
		json_merge_patch(
			json_object(
				'timeUnixNano', l.timestamp::varchar,
				'observedTimeUnixNano', l.observed_timestamp::varchar,
				'severityNumber', l.severity_number,
				'severityText', l.severity_text,
				'body', (select values[1] from value_batch),
				'attributes', otlp_attributes(l.attribute_ids, (select values from converted_attributes)),
				'droppedAttributesCount', l.dropped_attributes_count,
				'flags', l.flags,
				'eventName', l.event_name),
			case when l.trace_id is null then json('{}') else json_object('traceId', trace_id_wire(l.trace_id)) end,
			case when l.span_id is null then json('{}') else json_object('spanId', span_id_wire(l.span_id)) end
		) as document
	from selected l
),
scope_data as (
	select resource_id, resource_schema_url,
		json_object(
			'scope', otlp_scope(scope_id, (select values from converted_attributes)),
			'logRecords', [document],
			'schemaUrl', scope_schema_url) as document
	from log_documents
),
resource_data as (
	select json_object(
		'resource', otlp_resource(resource_id, (select values from converted_attributes)),
		'scopeLogs', [document],
		'schemaUrl', resource_schema_url) as document
	from scope_data
)
select otlp_document_text(json_object('resourceLogs', [document]))
from resource_data
