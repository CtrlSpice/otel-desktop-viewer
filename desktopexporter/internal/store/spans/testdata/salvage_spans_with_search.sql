
		with recursive
		search_params as (select try_cast(? as uuid) as trace_id),

		-- This trace's spans, isolated once, before the walk begins.
		--
		-- The recursive arm below runs once per level of the tree, and a
		-- recursive CTE cannot use an index on its working table: DuckDB
		-- re-joins whatever relation the arm names on every iteration. Naming
		-- `spans` there means each level hash-joins the entire table, so the
		-- cost of fetching one trace tracks how much telemetry the store holds
		-- rather than how big the trace is -- a point lookup priced as a scan.
		-- Measured on a 2.3M-span store, fetching a 159-span trace 14 levels
		-- deep: 54ms naming `spans`, 5ms naming this CTE, same rows out.
		--
		-- `materialized` is the load-bearing word. Without it DuckDB is free to
		-- inline the definition into each reference, which puts the full-table
		-- scan back exactly where it was removed from.
		trace_spans as materialized (
			select s.trace_id, s.span_id, s.parent_span_id, s.start_time
			from spans s, search_params
			where s.trace_id = search_params.trace_id
		),

		-- Sibling order, decided once, before the walk.
		--
		-- A span's rank among its siblings is a property of the trace, not of
		-- the traversal: it depends only on parent and start time, both known
		-- before the first row is walked. Computing it with a window inside
		-- the recursive arm instead re-runs a WINDOW operator once per level
		-- of the tree, paying full operator setup each time to rank a handful
		-- of siblings, and that cost is set by tree depth rather than by
		-- anything the query is being asked for.
		--
		-- It dominated. Profiled on a 122k-span store fetching a 159-span
		-- trace 14 levels deep, WINDOW was 27.9ms of a 46ms query -- against
		-- 0.8ms for the sequential scan over all 117,618 spans. Ranking once
		-- here leaves a single WINDOW over the trace's own rows.
		--
		-- Traversal order is unchanged, and that is checked rather than
		-- assumed: same rows, same depths, and the md5 of the span ids in
		-- sort_path order is identical either way.
		ranked as materialized (
			select t.*,
				row_number() over (
					partition by t.parent_span_id order by t.start_time
				) as sibling_rank,
				row_number() over (order by
					case when t.parent_span_id is null then 0 else 1 end,
					t.start_time
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
		matched_spans as (
			select s.span_id
			from search_params, spans s
		join resources r on r.id = s.resource_id
		join scopes sc on sc.id = s.scope_id
			where s.trace_id = search_params.trace_id AND (list_contains(s.attribute_ids, '9f1f8762-3dcf-10ee-4b04-db74f7de5b59'::uuid))
		),

		-- Spans the walk above could not reach, recovered best-effort.
		--
		-- Unreachable means the span's parent is present but the span is not
		-- descended from any root, which -- with one parent per span -- means
		-- it sits on a cycle. Nothing outside points into a cycle, so there is
		-- no edge to follow in; an entry has to be chosen.
		--
		-- Every unreached span is seeded as its own entry rather than
		-- identifying stranded components first, which would need real graph
		-- work for no gain: a span reached from several entries is deduped
		-- below, keeping the one from the earliest entry. For a lone A<->B
		-- cycle that yields A at depth 0 and B beneath it, which is the shape
		-- a reader can actually follow.
		salvage_seed as materialized (
			select r.*, row_number() over (order by r.start_time, r.span_id) as entry_rank
			from ranked r
			where r.span_id not in (select span_id from spans_tree)
		),

		-- The walk that may enter a cycle, so it carries its own ancestry and
		-- refuses to revisit. DuckDB has no CYCLE clause; this is the pattern
		-- its docs prescribe for traversing a graph that may contain one.
		salvage_walk as (
			select sd.span_id, sd.parent_span_id, sd.trace_id, sd.start_time,
				sd.entry_rank, 0 as depth, [sd.span_id] as visited
			from salvage_seed sd

			union all

			select r.span_id, r.parent_span_id, r.trace_id, r.start_time,
				sw.entry_rank, sw.depth + 1, list_append(sw.visited, r.span_id)
			from salvage_seed r
			join salvage_walk sw on r.parent_span_id = sw.span_id
			where list_position(sw.visited, r.span_id) is null
		),

		-- One placement per span: the earliest entry that reaches it.
		salvaged as materialized (
			select span_id, parent_span_id, trace_id, start_time, depth, entry_rank
			from salvage_walk
			qualify row_number() over (
				partition by span_id order by entry_rank, depth
			) = 1
		),

		-- The two walks, unioned, with the flags the UI needs.
		--
		-- cycle_point marks the span whose parent link is the lie: it is a
		-- display root of a salvaged chain whose own parent turns up further
		-- down that chain. Sorting salvaged trees after every real root keeps
		-- them out of the way of a trace that is otherwise fine.
		walked as (
			select trace_id, span_id, parent_span_id, start_time, depth, sort_path,
				false as salvaged, false as cycle_point
			from spans_tree

			union all

			select sv.trace_id, sv.span_id, sv.parent_span_id, sv.start_time, sv.depth,
				array[1000000 + sv.entry_rank::int] ||
					case when sv.depth = 0 then []::int[] else array[sv.depth] end,
				true,
				sv.depth = 0 and sv.parent_span_id in (select span_id from salvaged)
			from salvaged sv
		),

		-- The walk's result joined back to its payload, once.
		tree as materialized (
			select st.depth, st.sort_path, st.salvaged, st.cycle_point,
				s.span_id, s.parent_span_id, s.trace_id, s.trace_state, s.name, s.kind,
				s.flags,
				s.start_time, s.end_time, s.resource_id, s.scope_id, s.attribute_ids,
				s.dropped_attributes_count, s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message
			from walked st
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
				to_json(list(event_json(e, ea.attrs) order by e.timestamp)) as events
			from events e
			left join event_attrs ea on ea.id = e.id
			where e.span_id in (select span_id from tree)
			group by e.span_id
		),

		link_data as (
			select l.span_id,
				json_group_array(link_json(l, la.attrs)) as links
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
		-- Counted once and referenced twice: the response carries it for the
		-- client, and it comes back as its own column so the caller can branch
		-- without parsing. Written as two inline subqueries it was evaluated
		-- twice, which measured at ~11ms on a 245k-span store -- not the free
		-- subtraction of two materialised counts it looks like.
		unplaced_count as (
			select (select count(*) from trace_spans) - (select count(*) from tree) as n
		),

		trace_start as (
			select min(start_time) as t from tree
		),

		ordered_spans as (
			-- salvaged/cyclePoint ride an outer merge patch rather than the
			-- object itself, so they cost nothing on the overwhelming majority
			-- of traces where nothing is salvaged. Emitting two false booleans
			-- per span would put ~30 bytes on every row of a 5,735-span trace
			-- to describe a condition that almost never holds.
			select json_merge_patch(
				json_object(
					'spanData', span_data_json(
						ts, sa.attrs, ed.events, ld.links,
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
			join resource_data rd on rd.id = ts.resource_id
			join scope_data scd on scd.id = ts.scope_id
			left join span_attrs sa on sa.id = ts.span_id
			left join matched_spans ms on ts.span_id = ms.span_id
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
				-- Spans the walk could not place under any root.
				--
				-- A span becomes a root when its parent is absent from the
				-- trace, so anything left unreached still has a parent present
				-- -- which, with at most one parent per span, means it sits on
				-- a cycle. Malformed input rather than anything OTLP permits,
				-- and it cannot hang the walk, because a cycle member never
				-- qualifies as a root and the walk only ever descends into
				-- children. It is dropped instead, and dropping it silently is
				-- the part worth fixing: the trace renders short with nothing
				-- saying so.
				--
				-- Free to compute: both counts are already materialised.
				'unplacedSpanCount', (select n from unplaced_count),
				'spans', coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace,
		-- Same shape as search_spans so one scan path serves both. After
		-- salvage this should be 0; anything left is a span even the
		-- cycle-aware walk could not reach, which would be a bug worth seeing.
		(select n from unplaced_count) as unplaced
		from ordered_spans
	