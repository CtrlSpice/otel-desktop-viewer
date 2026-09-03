with
search_params as (
	select try_cast(? as uuid) as trace_id
),

trace_spans as materialized (
	select s.*
	from spans s, search_params
	where s.trace_id = search_params.trace_id
),

dict_map as materialized (
	select map(list(id), list({
		'j': json_object('key', key, 'value', value, 'type', type::varchar)
	})) as m
	from attributes
	where id in (
		select unnest(attribute_ids) from trace_spans
		union select unnest(e.attribute_ids) from events e
			where exists (select 1 from trace_spans s
				where s.trace_id = e.trace_id and s.span_id = e.span_id)
		union select unnest(l.attribute_ids) from links l
			where exists (select 1 from trace_spans s
				where s.trace_id = l.trace_id and s.span_id = l.span_id)
	)
),

span_attrs as (
	select s.trace_id, s.span_id, attrs_mapped(s.attribute_ids, dm.m) as attrs
	from trace_spans s, dict_map dm
	where len(s.attribute_ids) > 0
),

event_attrs as (
	select e.id, attrs_mapped(e.attribute_ids, dm.m) as attrs
	from events e, dict_map dm
	where exists (select 1 from trace_spans s
			where s.trace_id = e.trace_id and s.span_id = e.span_id)
		and len(e.attribute_ids) > 0
),

link_attrs as (
	select l.id, attrs_mapped(l.attribute_ids, dm.m) as attrs
	from links l, dict_map dm
	where exists (select 1 from trace_spans s
			where s.trace_id = l.trace_id and s.span_id = l.span_id)
		and len(l.attribute_ids) > 0
),

event_data as (
	select e.trace_id, e.span_id,
		to_json(list(event_json(e, ea.attrs) order by e.timestamp)) as events
	from events e
	left join event_attrs ea on ea.id = e.id
	where exists (select 1 from trace_spans s
		where s.trace_id = e.trace_id and s.span_id = e.span_id)
	group by e.trace_id, e.span_id
),

link_data as (
	select l.trace_id, l.span_id,
		to_json(list(link_json(l, la.attrs))) as links
	from links l
	left join link_attrs la on la.id = l.id
	where exists (select 1 from trace_spans s
		where s.trace_id = l.trace_id and s.span_id = l.span_id)
	group by l.trace_id, l.span_id
),

resource_data as (
	select r.id, r.seq, resource_json(r.attribute_ids, r.dropped_attributes_count) as obj
	from resources r
	where r.id in (select resource_id from trace_spans)
),

scope_data as (
	select sc.id, sc.seq,
		scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count) as obj
	from scopes sc
	where sc.id in (select scope_id from trace_spans)
),

flat_rows as (
	select json_object(
			'spanID', span_id_wire(s.span_id),
			'parentSpanID', case when s.parent_span_id is null
				then null else span_id_wire(s.parent_span_id) end,
			'traceState', s.trace_state,
			'flags', s.flags,
			'name', s.name,
			'kind', s.kind,
			'startTime', s.start_time::varchar,
			'endTime', s.end_time::varchar,
			'attributes', coalesce(sa.attrs, json('[]')),
			'events', coalesce(ed.events, json('[]')),
			'links', coalesce(ld.links, json('[]')),
			'resourceRef', rd.seq::varchar,
			'scopeRef', scd.seq::varchar,
			'droppedAttributesCount', s.dropped_attributes_count,
			'droppedEventsCount', s.dropped_events_count,
			'droppedLinksCount', s.dropped_links_count,
			'statusCode', s.status_code,
			'statusMessage', s.status_message
		) as row_json
	from trace_spans s
	join resource_data rd on rd.id = s.resource_id
	join scope_data scd on scd.id = s.scope_id
	left join span_attrs sa on sa.trace_id = s.trace_id and sa.span_id = s.span_id
	left join event_data ed on ed.trace_id = s.trace_id and ed.span_id = s.span_id
	left join link_data ld on ld.trace_id = s.trace_id and ld.span_id = s.span_id
)

select case
	when not exists (select 1 from trace_spans) then null
	else cast(json_object(
		'format', 'odv.trace-waterfall.flat.v1',
		'traceID', trace_id_wire((select trace_id from search_params)),
		'resources', coalesce(
			(select json_group_object(seq::varchar, obj) from resource_data),
			json('{}')
		),
		'scopes', coalesce(
			(select json_group_object(seq::varchar, obj) from scope_data),
			json('{}')
		),
		'rows', coalesce(
			(select to_json(list(row_json)) from flat_rows),
			json('[]')
		)
	) as varchar)
end as trace
