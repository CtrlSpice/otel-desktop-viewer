
		with recursive

		-- Materialize the trace subset so each recursive level avoids rejoining
		-- the complete spans table.
		trace_spans as materialized (
			select s.trace_id, s.span_id, s.parent_span_id, s.start_time
			from spans s
			where s.trace_id = try_cast($1 as uuid)
		),

		-- Rank roots and siblings once; window functions inside the recursive arm
		-- would run at every tree level.
		ranked as materialized (
			select t.*,
				row_number() over (
					partition by t.parent_span_id order by t.start_time, t.span_id
				) as sibling_rank,
				row_number() over (order by
					case when t.parent_span_id is null then 0 else 1 end,
					t.start_time, t.span_id
				) as root_rank
			from trace_spans t
		),

		spans_tree as (
			select
				r.trace_id, r.span_id, r.parent_span_id, r.start_time,
				0 as depth,
				array[0, r.root_rank] as sort_path
			from ranked r
			where r.parent_span_id is null
				or r.parent_span_id not in (select span_id from trace_spans)

			union all

			select
				r.trace_id, r.span_id, r.parent_span_id, r.start_time,
				st.depth + 1,
				st.sort_path || array[r.sibling_rank] as sort_path
			from ranked r
			join spans_tree st on r.parent_span_id = st.span_id
		),
		matched_spans as (
			select s.span_id
			from spans s
			where s.trace_id = try_cast($1 as uuid) AND ((variant_exists(s.attributes, $2) and otel_value_kind(s.attributes[$2]) = $3 and try_cast(s.attributes[$2] as varchar) = $4))
		),

		-- Seed each unreachable cycle member, then keep its earliest placement.
		salvage_seed as materialized (
			select r.*, row_number() over (order by r.start_time, r.span_id) as entry_rank
			from ranked r
			where r.span_id not in (select span_id from spans_tree)
		),

		-- Track ancestry to stop cycles. relative_path keeps each subtree contiguous.
		salvage_walk as (
			select sd.span_id, sd.parent_span_id, sd.trace_id, sd.start_time,
				sd.entry_rank, 0 as depth, []::bigint[] as relative_path,
				[sd.span_id] as visited
			from salvage_seed sd

			union all

			select r.span_id, r.parent_span_id, r.trace_id, r.start_time,
				sw.entry_rank, sw.depth + 1,
				sw.relative_path || array[r.sibling_rank],
				list_append(sw.visited, r.span_id)
			from salvage_seed r
			join salvage_walk sw on r.parent_span_id = sw.span_id
			where list_position(sw.visited, r.span_id) is null
		),

		-- One placement per span: the earliest entry that reaches it.
		salvaged as materialized (
			select span_id, parent_span_id, trace_id, start_time, depth, entry_rank,
				relative_path
			from salvage_walk
			qualify row_number() over (
				partition by span_id order by entry_rank, depth
			) = 1
		),

		-- cycle_point marks a display root whose reported parent appears later in
		-- the same salvage chain. Salvaged trees sort after normal roots.
		walked as (
			select trace_id, span_id, parent_span_id, start_time, depth, sort_path,
				false as salvaged, false as cycle_point
			from spans_tree

			union all

			select sv.trace_id, sv.span_id, sv.parent_span_id, sv.start_time, sv.depth,
				array[1, sv.entry_rank] || sv.relative_path,
				true,
				sv.depth = 0 and exists (
					select 1 from salvaged p
					where p.entry_rank = sv.entry_rank
						and p.span_id = sv.parent_span_id
				)
			from salvaged sv
		),

		-- Join traversed IDs to payloads once.
		tree as materialized (
			select st.depth, st.sort_path, st.salvaged, st.cycle_point,
				s.span_id, s.parent_span_id, s.trace_id, s.trace_state, s.name, s.kind,
				s.flags,
				s.start_time, s.end_time, s.resource, s.scope, s.attributes,
				s.dropped_attributes_count, s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message
			from walked st
			join spans s on s.trace_id = st.trace_id and s.span_id = st.span_id
		),

		event_data as (
			select e.trace_id, e.span_id,
				to_json(list(event_json(e, attribute_document(e.attributes)) order by e.timestamp, e.ordinal)) as events
			from events e
			where exists (select 1 from tree t
					where t.trace_id = e.trace_id and t.span_id = e.span_id)
			group by e.trace_id, e.span_id
		),

		link_data as (
			select l.trace_id, l.span_id,
				to_json(list(link_json(l, attribute_document(l.attributes)) order by l.ordinal)) as links
			from span_link_summaries(try_cast($1 as uuid)) l
			where exists (select 1 from tree t
					where t.trace_id = l.trace_id and t.span_id = l.span_id)
			group by l.trace_id, l.span_id
		),

		-- Build resource and scope JSON once per distinct owner.
		resource_data as (
			select resource.id as id, row_number() over (order by resource.id) as seq,
				json_object('attributes', attribute_document(resource.attributes),
					'droppedAttributesCount', resource.dropped_attributes_count) as obj
			from (select distinct resource from tree)
		),

		scope_data as (
			select scope.id as id, row_number() over (order by scope.id) as seq,
				json_object('name', scope.name, 'version', scope.version,
					'attributes', attribute_document(scope.attributes),
					'droppedAttributesCount', scope.dropped_attributes_count) as obj
			from (select distinct scope from tree)
		),

		-- Count spans still unreachable after salvage.
		unplaced_count as (
			select (select count(*) from trace_spans) - (select count(*) from tree) as n
		),

		trace_start as (
			-- Cross-host clock skew can place a child before its parent.
			select min(start_time) as t from tree
		),

		ordered_spans as (
			-- Omit salvage fields from normal spans.
			select json_merge_patch(
				json_object(
					'spanData', span_data_json(
						ts, attribute_document(ts.attributes), ed.events, ld.links,
						rd.seq, scd.seq, (select t from trace_start)
					),
					'depth', ts.depth,
					'matched', case when ms.span_id is not null then true else false end
				),
				case when ts.salvaged
					then json_object('salvaged', true, 'cyclePoint', ts.cycle_point)
					else json('{}')
				end
			) as span_json,
				ts.sort_path
			from tree ts
			join resource_data rd on rd.id = ts.resource.id
			join scope_data scd on scd.id = ts.scope.id
			left join matched_spans ms on ts.span_id = ms.span_id
			left join event_data ed on ts.trace_id = ed.trace_id and ts.span_id = ed.span_id
			left join link_data ld on ts.trace_id = ld.trace_id and ts.span_id = ld.span_id
		)

		select case
			when not exists (select 1 from spans where trace_id = try_cast($1 as uuid))
				then null
			else cast(json_object(
				'traceID', trace_id_wire(try_cast($1 as uuid)),
				-- Absolute nanoseconds as decimal text; span starts are offsets from it.
				'traceStart', (select t from trace_start)::varchar,
				-- Distinct resources and scopes keyed by sequence reference.
				'resources', coalesce((select json_group_object(seq::varchar, obj) from resource_data), json('{}')),
				'scopes', coalesce((select json_group_object(seq::varchar, obj) from scope_data), json('{}')),
				-- Any remaining unreachable span indicates a salvage defect.
				'unplacedSpanCount', (select n from unplaced_count),
				'spans', coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace,
		-- Keep the get_trace_view result shape; salvage should leave zero unplaced.
		(select n from unplaced_count) as unplaced
		from ordered_spans
