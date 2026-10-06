
		with recursive
		{{.CTEs}},

		-- Materialize the trace subset so each recursive level avoids rejoining
		-- the complete spans table.
		trace_spans as materialized (
			select s.trace_id, s.span_id, s.parent_span_id, s.start_time
			from spans s, search_params
			where s.trace_id = search_params.trace_id
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
		){{.MatchedCTE}},

		-- Join traversed IDs to payloads once.
		tree as materialized (
			select st.depth, st.sort_path,
				s.span_id, s.parent_span_id, s.trace_id, s.trace_state, s.name, s.kind,
				s.flags,
				s.start_time, s.end_time, s.resource_id, s.scope_id, s.attribute_ids,
				s.dropped_attributes_count, s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message
			from spans_tree st
			join spans s on s.trace_id = st.trace_id and s.span_id = st.span_id
		),

		-- Build one map containing only attribute IDs referenced by this trace.
		dict_map as materialized (
			select map(list(id), list({
				'k': key,
				'i': id,
				'j': json_object('id', id::varchar, 'key', key, 'value', value)
			})) as m
			from attributes
			where id in (
				select unnest(attribute_ids) from tree
				union select unnest(e.attribute_ids) from events e
					where exists (select 1 from tree t
					where t.trace_id = e.trace_id and t.span_id = e.span_id)
				union select unnest(l.attribute_ids) from links l
					where exists (select 1 from tree t
					where t.trace_id = l.trace_id and t.span_id = l.span_id)
			)
		),

		-- Resolve each owner by map probe; attrs_mapped preserves (key, id) order.
		span_attrs as (
			select ts.trace_id, ts.span_id as id, attrs_mapped(ts.attribute_ids, dm.m) as attrs
			from tree ts, dict_map dm
			where len(ts.attribute_ids) > 0
		),

		event_attrs as (
			select e.id, attrs_mapped(e.attribute_ids, dm.m) as attrs
			from events e, dict_map dm
			where exists (select 1 from tree t
					where t.trace_id = e.trace_id and t.span_id = e.span_id)
				and len(e.attribute_ids) > 0
		),

		link_attrs as (
			select l.id, attrs_mapped(l.attribute_ids, dm.m) as attrs
			from links l, dict_map dm
			where exists (select 1 from tree t
					where t.trace_id = l.trace_id and t.span_id = l.span_id)
				and len(l.attribute_ids) > 0
		),

		event_data as (
			select e.trace_id, e.span_id,
				to_json(list(event_json(e, ea.attrs) order by e.timestamp)) as events
			from events e
			left join event_attrs ea on ea.id = e.id
			where exists (select 1 from tree t
					where t.trace_id = e.trace_id and t.span_id = e.span_id)
			group by e.trace_id, e.span_id
		),

		link_data as (
			select l.trace_id, l.span_id,
				json_group_array(link_json(l, la.attrs)) as links
			from links l
			left join link_attrs la on la.id = l.id
			where exists (select 1 from tree t
					where t.trace_id = l.trace_id and t.span_id = l.span_id)
			group by l.trace_id, l.span_id
		),

		-- Build resource and scope JSON once per distinct owner.
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
						ts, sa.attrs, ed.events, ld.links,
						rd.seq, scd.seq, (select t from trace_start)
					),
				'depth', ts.depth,
				'matched', {{.MatchedExpr}}
			) as span_json,
				ts.sort_path
			from tree ts
			join resource_data rd on rd.id = ts.resource_id
			join scope_data scd on scd.id = ts.scope_id
			left join span_attrs sa on sa.trace_id = ts.trace_id and sa.id = ts.span_id
			{{.MatchedJoin}}
			left join event_data ed on ts.trace_id = ed.trace_id and ts.span_id = ed.span_id
			left join link_data ld on ts.trace_id = ld.trace_id and ts.span_id = ld.span_id
		)

		select case
			when not exists (select 1 from spans where trace_id = (select trace_id from search_params))
				then null
			else cast(json_object(
				'traceID', trace_id_wire((select trace_id from search_params)),
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
