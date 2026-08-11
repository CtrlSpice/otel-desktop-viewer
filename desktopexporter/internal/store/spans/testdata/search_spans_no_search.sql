
		with recursive
		search_params as (select try_cast(? as uuid) as trace_id),

		spans_tree as (
			select
				s.trace_id, s.span_id, s.parent_span_id, s.start_time,
				0 as depth,
				array[row_number() over (order by
					case when s.parent_span_id is null then 0 else 1 end,
					s.start_time
				)] as sort_path
			from spans s, search_params
			where s.trace_id = search_params.trace_id
				and (s.parent_span_id is null or s.parent_span_id not in (
					select span_id from spans where trace_id = search_params.trace_id
				))

			union all

			select
				s.trace_id, s.span_id, s.parent_span_id, s.start_time,
				st.depth + 1,
				st.sort_path || array[row_number() over (
					partition by st.span_id order by s.start_time
				)] as sort_path
			from spans s
			join spans_tree st on s.parent_span_id = st.span_id and s.trace_id = st.trace_id
		),

		-- The walk's result joined back to its payload, once.
		tree as materialized (
			select st.depth, st.sort_path,
				s.span_id, s.parent_span_id, s.trace_id, s.trace_state, s.name, s.kind,
				s.start_time, s.end_time, s.resource_id, s.scope_id, s.attribute_ids,
				s.dropped_attributes_count, s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message
			from spans_tree st
			join spans s on s.span_id = st.span_id
		),

		-- The attributes this trace references, as one MAP, probed by the three
		-- CTEs below.
		--
		-- Narrowed to the ids actually referenced, not the whole dictionary,
		-- and that is the difference between an optimisation and a liability.
		-- Building over every row costs the same as the group-by form it
		-- replaced once the dictionary is large, because the cost tracks total
		-- store content rather than the trace being fetched. Measured on a
		-- 5,735-span trace:
		--
		--	dictionary rows      whole-dict map      narrowed map
		--	1,286                0.036s / 0.118s     0.035s / 0.117s
		--	101,286              0.065s / 0.179s     0.036s / 0.118s
		--
		-- A season of F1 telemetry reaches ~1,286 distinct attributes, where
		-- the two are indistinguishable. A web service with a url.path per
		-- request reaches the second row on its first afternoon, and there the
		-- unnarrowed form gives back everything the map was for.
		--
		-- The trace's own working set is small and stays small: 73 distinct
		-- attributes for that 5,735-span trace.
		dict_map as materialized (
			select map(list(id), list({
				'k': key,
				'i': id,
				'j': json_object('key', key, 'value', value, 'type', type::varchar)
			})) as m
			from attributes
			where id in (
				select unnest(attribute_ids) from tree
				union select unnest(e.attribute_ids) from events e
					where e.span_id in (select span_id from tree)
				union select unnest(l.attribute_ids) from links l
					where l.span_id in (select span_id from tree)
			)
		),

		-- These three resolve attribute arrays by probing dict_map rather than
		-- by calling attrs_json, and that is deliberate.
		--
		-- attrs_json is a correlated subquery, which the planner materialises
		-- once per row in a per-row projection over a large table: 149ms for
		-- 4,868 spans. Rewriting it as unnest + join + group by brought that to
		-- 33ms but explodes each owner's array into rows only to collapse it
		-- again, and spends heavily on parallelism to do it -- 0.35s of CPU for
		-- 50ms of wall time on the whole query.
		--
		-- Probing a prebuilt map is both faster and cheaper: 37-40ms wall at
		-- 0.19s CPU. attrs_mapped orders by (key, id) exactly as attrs_json
		-- does, so the rendered JSON is byte-identical to both earlier forms.
		span_attrs as (
			select ts.span_id as id, attrs_mapped(ts.attribute_ids, dm.m) as attrs
			from tree ts, dict_map dm
			where len(ts.attribute_ids) > 0
		),

		event_attrs as (
			select e.id, attrs_mapped(e.attribute_ids, dm.m) as attrs
			from events e, dict_map dm
			where e.span_id in (select span_id from tree)
				and len(e.attribute_ids) > 0
		),

		link_attrs as (
			select l.id, attrs_mapped(l.attribute_ids, dm.m) as attrs
			from links l, dict_map dm
			where l.span_id in (select span_id from tree)
				and len(l.attribute_ids) > 0
		),

		event_data as (
			select e.span_id,
				to_json(list(json_object(
					'name', e.name,
					'timestamp', e.timestamp::varchar,
					'droppedAttributesCount', e.dropped_attributes_count,
					'attributes', coalesce(ea.attrs, json('[]'))
				) order by e.timestamp)) as events
			from events e
			left join event_attrs ea on ea.id = e.id
			where e.span_id in (select span_id from tree)
			group by e.span_id
		),

		link_data as (
			select l.span_id,
				json_group_array(json_object(
					'traceID', trace_id_wire(l.trace_id),
					'spanID', span_id_wire(l.linked_span_id),
					'traceState', l.trace_state,
					'droppedAttributesCount', l.dropped_attributes_count,
					'attributes', coalesce(la.attrs, json('[]'))
				)) as links
			from links l
			left join link_attrs la on la.id = l.id
			where l.span_id in (select span_id from tree)
			group by l.span_id
		),

		-- Resource and scope JSON is built once per *distinct owner*, which is
		-- the entire point of deduping them. Inlining resource_json/scope_json
		-- in the per-span projection re-resolved the same 24 resources 4,891
		-- times and cost two ~150ms operators.
		resource_data as (
			select r.id, r.seq, resource_json(r.attribute_ids, r.dropped_attributes_count) as obj
			from resources r
			where r.id in (select resource_id from tree)
		),

		scope_data as (
			select sc.id, sc.seq,
				scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count) as obj
			from scopes sc
			where sc.id in (select scope_id from tree)
		),

		-- The baseline every span offset is measured from.
		--
		-- min(start_time), not the root span's start: clocks across hosts are
		-- not synchronised, so a child can legitimately report an earlier start
		-- than its parent, and min() is also indifferent to whether the trace
		-- has a root at all -- which matters, since a trace whose parent is
		-- missing is displayed as rooted anyway.
		trace_start as (
			select min(start_time) as t from tree
		),

		ordered_spans as (
			select json_object(
					'spanData', json_object(
						-- No traceID: it is at the response root, and a
						-- single-trace response repeated it 32 bytes per span.
						'traceState', ts.trace_state,
						'spanID', span_id_wire(ts.span_id),
						'parentSpanID', case when ts.parent_span_id is not null then span_id_wire(ts.parent_span_id) end,
						'name', ts.name,
						'kind', ts.kind,
						-- Offset from traceStart, and duration from the span's
						-- own start. Deliberately not two offsets: an end
						-- offset inherits the trace's full magnitude however
						-- brief the span, while a duration stays small. It is
						-- also what the waterfall wants -- a bar is a position
						-- and a width, so the client stops subtracting on every
						-- render.
						'start', ts.start_time - (select t from trace_start),
						'dur', ts.end_time - ts.start_time,
						'attributes', coalesce(sa.attrs, json('[]')),
						'events', coalesce(ed.events, json('[]')),
						'links', coalesce(ld.links, json('[]')),
						-- References into the top-level maps. seq rather than
						-- the uuid: two 36-char ids per span is ~413KB on the
						-- reference trace, a small integer ~57KB. The uuids are
						-- storage identity and have no business on the wire.
						'r', rd.seq,
						's', scd.seq,
						'droppedAttributesCount', ts.dropped_attributes_count,
						'droppedEventsCount', ts.dropped_events_count,
						'droppedLinksCount', ts.dropped_links_count,
						'statusCode', ts.status_code,
						'statusMessage', ts.status_message
					),
				'depth', ts.depth,
				'matched', true
			) as span_json,
				ts.sort_path
			from tree ts
			join resource_data rd on rd.id = ts.resource_id
			join scope_data scd on scd.id = ts.scope_id
			left join span_attrs sa on sa.id = ts.span_id
			
			left join event_data ed on ts.span_id = ed.span_id
			left join link_data ld on ts.span_id = ld.span_id
		)

		select case
			when not exists (select 1 from spans where trace_id = (select trace_id from search_params))
				then null
			else cast(json_object(
				'traceID', trace_id_wire((select trace_id from search_params)),
				-- Absolute ns as a string; only this one needs the full
				-- magnitude, and only the detail panel reads it, as
				-- BigInt(traceStart) + BigInt(start).
				'traceStart', (select t from trace_start)::varchar,
				-- Each distinct resource and scope once, keyed by seq. On the
				-- reference trace this is 24 resources and 1 scope against
				-- 5,735 spans that previously carried a full copy each,
				-- which was over half the response.
				'resources', coalesce((select json_group_object(seq::varchar, obj) from resource_data), json('{}')),
				'scopes', coalesce((select json_group_object(seq::varchar, obj) from scope_data), json('{}')),
				'spans', coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace
		from ordered_spans
	