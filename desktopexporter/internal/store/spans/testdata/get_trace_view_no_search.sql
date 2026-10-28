
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
				array[r.root_rank] as sort_path
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

		-- Join traversed IDs to payloads once.
		tree as materialized (
			select st.depth, st.sort_path,
				s.span_id, s.parent_span_id, s.trace_id, s.trace_state, s.name, s.kind,
				s.flags,
				s.start_time, s.end_time, s.resource, s.scope, s.attributes,
				s.dropped_attributes_count, s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message
			from spans_tree st
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

		-- Count spans unreachable from a root once for JSON and caller branching.
		unplaced_count as (
			select (select count(*) from trace_spans) - (select count(*) from tree) as n
		),

		trace_start as (
			-- Cross-host clock skew can place a child before its parent.
			select min(start_time) as t from tree
		),

		ordered_spans as (
			select json_object(
					'spanData', span_data_json(
						ts, attribute_document(ts.attributes), ed.events, ld.links,
						rd.seq, scd.seq, (select t from trace_start)
					),
				'depth', ts.depth,
				'matched', true
			) as span_json,
				ts.sort_path
			from tree ts
			join resource_data rd on rd.id = ts.resource.id
			join scope_data scd on scd.id = ts.scope.id
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
				-- Unreachable spans have a present parent and therefore belong to a cycle.
				'unplacedSpanCount', (select n from unplaced_count),
				'spans', coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace,
		-- Separate column lets the caller select salvage without parsing JSON.
		(select n from unplaced_count) as unplaced
		from ordered_spans
