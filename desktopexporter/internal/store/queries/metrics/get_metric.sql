
		with input as (
			select ?::uuid as stream_id,
				?::bigint as time_start,
				?::bigint as time_end,
				?::bigint as target_buckets,
				-- Which series the caller cares about. Empty means all of them,
				-- which is the unfiltered behaviour every existing caller gets.
				--
				-- Bound as varchar[] and cast in SQL rather than as a uuid list:
				-- the driver corrupts a bound []duckdb.UUID silently, matching
				-- nothing (see the uuid_binding canary in store/util).
				?::varchar[] as series_ids,
				-- Which quantiles to compute per histogram datapoint. Empty skips the
				-- work entirely, so a caller drawing no overlays pays nothing.
				--
				-- Computed here rather than shipped as bucket vectors for the client
				-- to reduce: three numbers per series per bucket instead of a
				-- forty-element array, and one implementation of the arithmetic.
				?::double[] as quantiles,
				-- The viewer's UTC offset in nanoseconds, so bucket boundaries fall
				-- where the reader's calendar says a day breaks rather than where the
				-- epoch does. 0 aligns to UTC, which is what a caller that does not
				-- care should send.
				--
				-- A query parameter, not URL state: it comes from a user preference,
				-- so a shared link renders in the recipient's calendar rather than
				-- the sender's.
				?::bigint as tz_offset_ns,
				-- Whether the caller's window is a request or the absence of one.
				--
				-- The reduction divides a span by target_buckets, and the span it
				-- divided was always the *requested* one. Ask for "All" -- epoch to
				-- now, fifty-six years -- and a bucket is 414 days wide, so an
				-- entire two-hour race merges into one datapoint per series. The
				-- chart was not misdrawing a fine reduction; it was drawing the one
				-- bucket it was sent.
				--
				-- A caller that never chose a window is not asking about the fifty-
				-- six years, so with this set the span comes from the data instead
				-- (see data_extent). The decision stays with the caller, which owns
				-- the time picker and knows whether a window was chosen; the store
				-- only obeys. Inferring it here from time_start = 0 would put the
				-- same rule in two languages, to drift apart later.
				?::boolean as fit_to_data,
				-- How many buckets the Sum / Average / Rate views aggregate onto.
				--
				-- Deliberately not target_buckets. The election's job is to keep
				-- the line's shape while thinning it; the view's job is chart
				-- resolution for a different chart. Welding them would make a
				-- change to one silently retune the other.
				?::bigint as view_buckets
		),
		stream as (
			select s.* from metric_streams s, input
			where s.id = input.stream_id
		),
		matched_ingests as (
			select m.* from metric_ingests m, input
			where m.stream_id = input.stream_id
			  and exists (
				select 1 from datapoints d
				where d.metric_ingest_id = m.id
				  and d.timestamp >= input.time_start and d.timestamp <= input.time_end
			  )
		),
		-- Datapoints inherit aggregation_temporality / is_monotonic from
		-- the stream so the per-type JSON projection below doesn't need
		-- a per-row join.
		-- resource_id rides along for the per-batch resource of each datapoint.
		-- It is NOT what groups a series: `resources` is content-addressed, so
		-- one instance that gets enriched mid-stream owns several rows there,
		-- and grouping on it split a single series across them. A join rather
		-- than a denormalized column on datapoints: it is a primary-key lookup
		-- from metric_ingest_id, and datapoints is the largest table here.
		filtered_dps as (
			select d.*,
				s.metric_type as metric_type,
				s.aggregation_temporality as aggregation_temporality,
				s.is_monotonic as is_monotonic
			from datapoints d, input, stream s
			where d.stream_id = input.stream_id
			  and d.timestamp >= input.time_start and d.timestamp <= input.time_end
			  -- Narrowing here rather than after the merge: the reduction and
			  -- every alignment stage downstream then run over only the series
			  -- asked for, instead of merging series the caller will discard.
			  -- NULL means "no filter", an empty list means "no series". Those
			  -- are different questions and the client already distinguishes
			  -- them: its visible-key set is empty when a user has unticked
			  -- every series, and absent when the concept does not apply.
			  -- Collapsing empty to "all" would render every series at the
			  -- exact moment the user asked for none.
			  and (input.series_ids is null
			       or list_contains(input.series_ids, d.series_id::varchar))
		),
		-- The dp_attrs_agg and exemplar_attrs CTEs are gone: attrs_json
		-- resolves each row's id array in place, so there is nothing to
		-- pre-aggregate and join back.
		exemplars_agg as (
			select e.datapoint_id, json_group_array(exemplar_json(e)) as exemplars
			from exemplars e
			where e.datapoint_id in (select id from filtered_dps)
			group by e.datapoint_id
		),
		-- Per-ingest latest datapoint timestamp over the queried window
		-- -- the recency proxy we use to pick a "representative" ingest
		-- for description / dropped counts. These per-batch fields can
		-- drift across ingests; we prefer the most recently-observed
		-- sender's view (newest data, not newest wall-clock arrival).
		ingest_latest_dp as (
			select metric_ingest_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by metric_ingest_id
		),
		-- Most recent matched ingest is the source of variable-but-
		-- non-identifying fields (description, dropped counts).
		representative as (
			select mi.* from matched_ingests mi
			inner join ingest_latest_dp ild on ild.metric_ingest_id = mi.id
			order by ild.last_dp_ts desc nulls last
			limit 1
		),
		-- Resource and scope come from the representative ingest, the same
		-- row the dropped counts come from.
		--
		-- They used to be aggregated over *all* matched ingests with no
		-- DISTINCT, so a metric with 360 batches emitted 360 duplicate copies
		-- of each resource attribute while its droppedAttributesCount came
		-- from one ingest -- an asymmetry that only looked harmless because
		-- the frontend deduped by key on render. Taking both from the same
		-- row fixes it, and the dedupe makes it free: every batch from the
		-- same sender now points at one resources row anyway.
		representative_owners as (
			select r.attribute_ids as resource_attribute_ids,
			       r.dropped_attributes_count as resource_dropped,
			       sc.attribute_ids as scope_attribute_ids,
			       sc.dropped_attributes_count as scope_dropped
			from representative rep
			join resources r on r.id = rep.resource_id
			join scopes sc on sc.id = rep.scope_id
		),
		-- One row per (metric, attribute-set) -- i.e. per OTel stream.
		-- The attribute set itself is owned by the stream (lifted out of
		-- the per-dp objects), and the dp objects inside are pure OTLP
		-- measurement payloads: timestamp, type-specific value fields,
		-- exemplars, flags. attrs_canonical is the grouping key; we
		-- coalesce NULL (no-attrs case) to "" so all attribute-less
		-- points collapse into one timeseries rather than scattering.
		--
		-- attributes_sample picks any one datapoint's attributes from
		-- this timeseries. Within a timeseries they're identical by
		-- construction -- series_id is content-derived from (stream,
		-- instance, attribute_ids), so the array cannot vary inside a group.
		--
		-- any_value wraps the *array*, not the resolved JSON. Written the
		-- other way round the macro sits inside the aggregate, so it runs once
		-- per datapoint and every result but one is thrown away. attrs_json is
		-- a correlated subquery, which makes that expensive in the worst way:
		-- measured on one stream of the reference capture, 220,913 datapoints
		-- collapsing to 220 series,
		--
		--	any_value(attrs_json(ids))   0.687s wall, 7.20s CPU
		--	attrs_json(any_value(ids))   0.021s wall, 0.05s CPU
		--
		-- 33x wall and 141x CPU, for identical output. Aggregate first, then
		-- resolve once per group.
		-- M4 reduction: at most four datapoints per series per time bucket --
		-- the earliest, the latest, the smallest and the largest.
		--
		-- Chosen over the client's LTTB because for a chart of a given width
		-- the line drawn from these points is *identical* to the line drawn
		-- from every point: the extremes of each pixel column are always
		-- present, so nothing that would have been visible is dropped. LTTB
		-- preserves shape but is a sample -- a spike survives only if its
		-- triangle is large enough.
		--
		-- Skipped entirely when target_buckets is null, which is the default:
		-- reduction is opt-in, and a caller that wants every datapoint still
		-- gets every datapoint.
		-- Columns, not subqueries. bucket_width_ns filters a list with a
		-- lambda, and a subquery passed as an argument is inlined into that
		-- lambda body, where DuckDB rejects it: "subqueries in lambda
		-- expressions are not supported". Reading from `input` as a relation
		-- keeps the arguments plain.
		-- The extent the data actually occupies inside the requested window,
		-- and the span the reduction divides.
		--
		-- +1 because both ends are inclusive: a stream with a single datapoint
		-- spans one nanosecond, not zero, and a zero span would divide to a
		-- zero-width bucket. coalesce covers the empty result, where there is
		-- no extent to fit and the requested window is all there is.
		--
		-- Only consulted when the caller said its window was not a choice. An
		-- explicit window keeps dividing itself, so its buckets stay anchored
		-- to the window: panning slides data through fixed boundaries instead
		-- of re-cutting them per request, and a metric that stopped reporting
		-- keeps the empty space that says so.
		data_extent as (
			select min(timestamp) as min_ts, max(timestamp) as max_ts
			from filtered_dps
		),
		reduction_span as (
			select case
				when i.fit_to_data then coalesce(
					(select max_ts - min_ts + 1 from data_extent),
					i.time_end - i.time_start
				)
				else i.time_end - i.time_start
			end as span_ns
			from input i
		),
		reduction as (
			select case
				-- Gauge and Sum sample; histograms merge (see hist_merged).
				-- Sampling a histogram is not a reduction,
				-- it is data loss: each datapoint carries bucket *counts*, so
				-- keeping four per bucket discards the observations in all the
				-- rest. Measured on the reference stream, sampling took 275,196
				-- observations down to 729 -- quantiles and heatmap alike would
				-- be fiction, with nothing to indicate it.
				--
				-- The correct reduction for a histogram is a merge: add counts
				-- for Delta, last-minus-first for Cumulative. Until that exists,
				-- histograms return every datapoint however small a resolution
				-- is asked for. Slow beats wrong.
				when s.metric_type in ('Gauge', 'Sum')
					then bucket_width_ns(rs.span_ns, i.target_buckets)
				-- Histograms reduce by merging. Delta adds bucket counts;
				-- Cumulative subtracts the first of the bucket from the last,
				-- because each datapoint is a running total and adding them
				-- would count every observation once per datapoint.
				when s.metric_type in ('Histogram', 'ExponentialHistogram')
				     and s.aggregation_temporality in ('Delta', 'Cumulative')
					then bucket_width_ns(rs.span_ns, i.target_buckets)
			end as width_ns
			-- reduction_span as a relation, not a scalar subquery: the comment
			-- above applies to it for the same reason it applies to the window.
			from input i, stream s, reduction_span rs
		),

		-- Is this a histogram merge or a scalar election? They share the
		-- bucketing above and diverge here.
		reduction_kind as (
			select case
				when (select width_ns from reduction) is null then 'none'
				when s.metric_type in ('Histogram', 'ExponentialHistogram') then 'merge'
				else 'elect'
			end as kind
			from stream s
		),

		-- Bucket starts are absolute, not measured from the window: floor by
		-- the width so panning slides data through fixed buckets rather than
		-- re-cutting them on every request.
		-- Shift into the viewer's local time, floor, shift back -- the same
		-- three steps histogramBucketStart takes in TypeScript.
		--
		-- floor_div rather than //: integer division truncates toward zero, so
		-- a pre-epoch timestamp would floor the wrong way and land a datapoint
		-- one bucket late. The client comment flags the identical hazard for
		-- BigInt division.
		bucketed_dps as (
			select d.*,
				floor_div(d.timestamp + (select tz_offset_ns from input),
				          (select width_ns from reduction))
					* (select width_ns from reduction)
					- (select tz_offset_ns from input) as bucket_start
			from filtered_dps d
			where (select width_ns from reduction) is not null
		),

		-- Per-interval activity for a cumulative scalar series.
		--
		-- Each datapoint of a Cumulative Sum is a running total, so the activity
		-- in an interval is the difference from the previous datapoint of the
		-- same series. The first datapoint has no predecessor and produces no
		-- interval -- N readings describe N-1 intervals, which is what the null
		-- check drops.
		--
		-- A fall means the counter restarted between the two readings. The
		-- convention (Prometheus rate(), and the TypeScript this replaces) is to
		-- treat the current reading as the increment since the restart: the
		-- prior counter is gone, so it is the best reference available. The reset
		-- is reported rather than smoothed away, so the chart can mark it instead
		-- of drawing a cliff.
		--
		-- Computed before any reduction, which is the half the client could not
		-- do: it differenced *chart points*, which have already been through the
		-- M4 election, so its "previous point" was frequently not the previous
		-- datapoint and the delta silently spanned the gap.
		--
		-- isfinite for the same reason bucket_elected uses it: DuckDB orders NaN
		-- above infinity, and one NaN poisons every difference after it. A NaN
		-- the instrument really sent is still visible in the datapoint list,
		-- which is fetched unreduced -- the chart drops what it cannot draw, the
		-- record keeps what arrived.
		scalar_dps as (
			select d.series_id, d.timestamp,
				coalesce(d.double_value, d.int_value) as value
			from filtered_dps d
			where d.metric_type in ('Gauge', 'Sum')
			  and isfinite(coalesce(d.double_value, d.int_value))
		),
		scalar_lagged as (
			select s.*,
				lag(s.value) over (
					partition by s.series_id order by s.timestamp
				) as prev_value
			from scalar_dps s
		),
		scalar_deltas as (
			select series_id,
				timestamp,
				case when value < prev_value then value
				     else value - prev_value
				end as delta,
				value < prev_value as is_reset
			from scalar_lagged
			where prev_value is not null
		),

		-- The grid the scalar views aggregate on.
		--
		-- Absolute boundaries from the ladder, not span/N measured from each
		-- series' own first and last point. That per-series origin is what made
		-- two lines' "bucket 3" cover different intervals; the client's
		-- sharedBucketCount papers over it by handing both paths the same bucket
		-- *count*, and its comment concedes that only lines up because the
		-- series happen to share a scrape cadence.
		scalar_view_grid as (
			-- reduction_span as a relation, not a scalar subquery: bucket_width_ns
			-- filters a list with a lambda, and a subquery argument is inlined
			-- into that lambda body where DuckDB rejects it. The reduction CTE
			-- above takes the same care for the same reason.
			select bucket_width_ns(rs.span_ns, i.view_buckets) as width_ns
			from input i, reduction_span rs
		),
		scalar_view_bucketed as (
			select d.series_id,
				floor_div(d.timestamp + (select tz_offset_ns from input),
				          (select width_ns from scalar_view_grid))
					* (select width_ns from scalar_view_grid)
					- (select tz_offset_ns from input) as bucket_start,
				d.value,
				sd.delta,
				sd.is_reset
			from scalar_dps d
			left join scalar_deltas sd
				on sd.series_id = d.series_id and sd.timestamp = d.timestamp
		),
		-- Each series' own first and last populated bucket.
		--
		-- Per series, not per response: a series that started late should not
		-- carry empty buckets for time before it existed, nor a series that
		-- stopped trail them afterwards. The boundaries stay shared -- only the
		-- extent differs -- so two series still line up where they overlap.
		scalar_view_extent as (
			select series_id,
				min(bucket_start) as first_bucket,
				max(bucket_start) as last_bucket
			from scalar_view_bucketed
			group by series_id
		),
		-- Every bucket between those ends, including the ones nothing landed in.
		--
		-- Interior gaps are information: a Sum or Rate of zero across an outage
		-- is the answer, and a chart that joined the two sides would draw
		-- straight through it. Leading and trailing empties are not, which is
		-- what the extent above trims.
		scalar_view_spine as (
			select e.series_id,
				unnest(range(e.first_bucket,
				             e.last_bucket + g.width_ns,
				             g.width_ns)) as bucket_start
			from scalar_view_extent e, scalar_view_grid g
		),
		-- One row per (series, bucket), carrying every view's answer.
		--
		-- An empty bucket comes out null rather than zero, and that falls out of
		-- SQL rather than being arranged: sum and avg over no rows are null. It
		-- is exactly the distinction the views need -- Sum and Rate read a gap
		-- as zero activity, Average has to skip it, because the mean of nothing
		-- is not nought. Emitting 0 here would bake one view's reading into all
		-- three. sample_count rides along so "no samples" stays legible.
		--
		-- All four are computed rather than the caller naming a view: they are
		-- aggregate functions over rows already grouped, so the extra ones cost
		-- almost nothing, and switching tabs then needs no round trip. The
		-- selection-dependent pooled lines are a different matter -- those
		-- recompute when the legend changes, as the histogram aggregate does.
		--
		-- Only rate differences a cumulative counter. Summing across series at
		-- time t means adding the running totals; delta-converting first would
		-- turn Sum into "events in this window" and Average into "mean
		-- increment" -- useful numbers, but not the ones those labels promise.
		scalar_view_agg as (
			select sp.series_id,
				sp.bucket_start,
				count(b.series_id) as sample_count,
				sum(b.value) as value_sum,
				avg(b.value) as value_avg,
				sum(b.delta) / ((select width_ns from scalar_view_grid) / 1e9) as rate,
				-- Did the counter restart inside this bucket? The rate view marks
				-- it, because a restart is the one place the number understates
				-- what happened and the chart should say so rather than draw a
				-- plausible dip.
				coalesce(bool_or(b.is_reset), false) as has_reset
			from scalar_view_spine sp
			left join scalar_view_bucketed b
				on b.series_id = sp.series_id
			   and b.bucket_start = sp.bucket_start
			group by sp.series_id, sp.bucket_start
		),
		scalar_views_agg as (
			select series_id,
				to_json(list(json_object(
					'bucketStart', bucket_start::varchar,
					'sampleCount', sample_count,
					'sum', value_sum,
					'avg', value_avg,
					'rate', rate,
					'hasReset', has_reset
				) order by bucket_start)) as views
			from scalar_view_agg
			group by series_id
		),

		-- isfinite: DuckDB orders NaN above infinity, so one NaN sample would
		-- win max() and elect itself as its bucket's representative, displacing
		-- a real value. It cannot be charted either way, so it is excluded from
		-- the election rather than allowed to win it.
		bucket_elected as (
			select
				series_id,
				bucket_start,
				arg_min(id, timestamp) as first_id,
				arg_max(id, timestamp) as last_id,
				arg_min(id, coalesce(double_value, int_value))
					filter (where isfinite(coalesce(double_value, int_value))) as min_id,
				arg_max(id, coalesce(double_value, int_value))
					filter (where isfinite(coalesce(double_value, int_value))) as max_id
			from bucketed_dps
			group by series_id, bucket_start
		),

		-- The ids that survive: the elected four per bucket, plus every
		-- datapoint carrying an exemplar.
		--
		-- Exemplars are the link from a metric to a trace, and election is
		-- driven by *value*, so the datapoints holding them are mostly not the
		-- ones M4 keeps. Dropping them would quietly gut trace correlation on
		-- exactly the dense streams this reduction exists for. They are sparse
		-- by construction, so keeping all of them costs little.
		retained_ids as (
			select unnest([first_id, last_id, min_id, max_id]) as id from bucket_elected
			union
			select d.id from filtered_dps d
			where exists (select 1 from exemplars e where e.datapoint_id = d.id)
		),

		-- Histogram merge, Delta only.
		--
		-- Adding bucket counts is exact: each datapoint covers its own
		-- interval, so a merged histogram yields the same quantiles and the
		-- same heatmap column as the datapoints it replaces. There is no
		-- fidelity trade here, only arithmetic -- which is why histograms merge
		-- rather than being sampled like scalars.
		--
		-- Exponential histograms need aligning first. Two histograms only add
		-- directly if they share a scale and an offset, and an SDK downscales a
		-- stream mid-flight as the observed range widens. So: downscale each to
		-- the coarsest scale in its bucket, left-pad each to the smallest
		-- offset, then add. On a stream whose scale never moves -- which is the
		-- common case, and the whole reference corpus -- every downscale is a
		-- no-op and costs nothing.
		hist_scaled as (
			select b.*,
				min(b.scale) over (partition by b.series_id, b.bucket_start) as target_scale
			from bucketed_dps b
			where (select kind from reduction_kind) = 'merge'
		),
		hist_downscaled as (
			select h.*,
				downscale_exp_buckets(h.positive_bucket_counts, h.positive_bucket_offset,
					h.scale - h.target_scale) as pos_d,
				downscale_exp_buckets(h.negative_bucket_counts, h.negative_bucket_offset,
					h.scale - h.target_scale) as neg_d
			from hist_scaled h
		),
		-- Only arrays holding buckets get a say in the alignment point: an
		-- empty array's offset points at no data, and letting it win the
		-- minimum pads the result out to an index nothing occupies.
		hist_aligned as (
			select d.*,
				min(case when len(d.pos_d.counts) > 0 then d.pos_d.offset end)
					over (partition by d.series_id, d.bucket_start) as pos_target_offset,
				min(case when len(d.neg_d.counts) > 0 then d.neg_d.offset end)
					over (partition by d.series_id, d.bucket_start) as neg_target_offset
			from hist_downscaled d
		),
		hist_padded as (
			select a.*,
				pad_left_to_offset(a.pos_d.counts, a.pos_d.offset,
					coalesce(a.pos_target_offset, a.pos_d.offset)) as pos_p,
				pad_left_to_offset(a.neg_d.counts, a.neg_d.offset,
					coalesce(a.neg_target_offset, a.neg_d.offset)) as neg_p
			from hist_aligned a
		),
		hist_merged as (
			select
				p.series_id,
				p.bucket_start,
				-- A real datapoint id, not a synthetic one: the last of the
				-- bucket. Keeps ?dp= links and datapoint selection working
				-- against something that exists.
				arg_max(p.id, p.timestamp) as id,
				-- The series' labels, which the merge would otherwise drop.
				--
				-- projected_dps unions this branch with filtered_dps `by name`,
				-- so a column missing here is not an error -- it arrives as
				-- NULL. attrs_json(NULL) is [], so every merged histogram series
				-- came back with no attributes and the legend labelled all
				-- twenty-one of them "default series".
				--
				-- any_value is exact rather than arbitrary: series_id is
				-- content-derived from (stream, instance, attribute_ids), so the
				-- array cannot vary within a group.
				any_value(p.attribute_ids) as attribute_ids,
				-- The bucket this row *is*, not the newest datapoint that went
				-- into it.
				--
				-- max(timestamp) put every merged row on a constituent's clock,
				-- so rows the store had grouped into one 30-second bucket came
				-- back on timestamps 5 seconds apart, and two series merged over
				-- the same bucket disagreed about when it was. The row is the
				-- bucket; its timestamp should say which one.
				--
				-- Safe because this path only runs when there is a reduction:
				-- bucketed_dps filters on width_ns being non-null, so
				-- bucket_start is never null here. The unreduced path returns
				-- real datapoints and keeps their own timestamps, which is
				-- right -- nothing was merged.
				--
				-- start_time stays the earliest constituent's rather than
				-- becoming bucket_start: it is OTLP's "when did this
				-- observation period begin", and the earliest is the honest
				-- answer for a group. Only the point on the time axis is the
				-- bucket's.
				p.bucket_start as timestamp,
				min(p.start_time) as start_time,
				any_value(p.metric_type) as metric_type,
				any_value(p.aggregation_temporality) as aggregation_temporality,
				any_value(p.flags) as flags,
				any_value(p.is_monotonic) as is_monotonic,
				-- Delta adds; Cumulative takes last minus first.
				--
				-- The alignment chain above has already put every datapoint in
				-- this bucket on a common scale and origin, so the earliest and
				-- latest are directly comparable and the subtraction is a
				-- straight element-wise difference.
				--
				-- diff_bucket_vectors returns NULL when any bucket would go
				-- negative, which means the counter restarted. The clamp then
				-- falls back to the later slice, because after a restart the
				-- later value *is* the activity since the restart. That is a
				-- different situation from failing to align, which is why the
				-- two are not allowed to share an exit.
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum(p.count)
					else greatest(max(p.count) - min(p.count), 0)
				end as count,
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum(p.sum)
					else greatest(max(p.sum) - min(p.sum), 0)
				end as sum,
				-- Explicit bounds: identical across the group or the merge is
				-- meaningless, and there is no rescale that reconciles them.
				any_value(p.explicit_bounds) as explicit_bounds,
				count(distinct p.explicit_bounds::varchar) as distinct_bounds,
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum_bucket_vectors(list(p.bucket_counts))
					else coalesce(
						diff_bucket_vectors(arg_max(p.bucket_counts, p.timestamp), arg_min(p.bucket_counts, p.timestamp)),
						arg_max(p.bucket_counts, p.timestamp)
					)
				end as bucket_counts,
				any_value(p.target_scale) as scale,
				max(p.zero_threshold) as zero_threshold,
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum(p.zero_count)
					else greatest(max(p.zero_count) - min(p.zero_count), 0)
				end as zero_count,
				any_value(coalesce(p.pos_target_offset, 0)) as positive_bucket_offset,
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum_bucket_vectors(list(p.pos_p))
					else coalesce(
						diff_bucket_vectors(arg_max(p.pos_p, p.timestamp), arg_min(p.pos_p, p.timestamp)),
						arg_max(p.pos_p, p.timestamp)
					)
				end as positive_bucket_counts,
				any_value(coalesce(p.neg_target_offset, 0)) as negative_bucket_offset,
				case when any_value(p.aggregation_temporality) = 'Delta'
					then sum_bucket_vectors(list(p.neg_p))
					else coalesce(
						diff_bucket_vectors(arg_max(p.neg_p, p.timestamp), arg_min(p.neg_p, p.timestamp)),
						arg_max(p.neg_p, p.timestamp)
					)
				end as negative_bucket_counts
			from hist_padded p
			group by p.series_id, p.bucket_start
		),

		-- Reconcile the merged zero threshold with the merged buckets.
		--
		-- hist_merged takes the largest zero_threshold of the group, because a
		-- merged histogram cannot claim to resolve values that one of its inputs
		-- did not. That leaves the input with the *smaller* threshold contributing
		-- buckets covering (its T, the merged T] -- a range the merged threshold
		-- now declares empty. Those buckets have to move into zero_count, or the
		-- datapoint says one thing in zero_threshold and another in its arrays.
		--
		-- Two CTEs rather than one because SQL cannot reference a select alias
		-- from the same select list, and calling the macro once per output column
		-- would evaluate it six times.
		--
		-- Explicit-bounds histograms fall through untouched: they carry no zero
		-- threshold, exp_zero_cutoff returns NULL for a null or non-positive one,
		-- and fold_below_cutoff treats a NULL cutoff as "fold nothing". No branch
		-- on metric_type is needed.
		--
		-- Mirrors mergeExpHistogramStreams in histogram-merge.ts, which computes
		-- the same cutoff for both signs from the same (threshold, scale) and adds
		-- both folded totals into zero_count.
		hist_folds as (
			select m.*,
				zero_fold(m.positive_bucket_counts, m.positive_bucket_offset,
					m.zero_threshold, m.scale) as pos_fold,
				zero_fold(m.negative_bucket_counts, m.negative_bucket_offset,
					m.zero_threshold, m.scale) as neg_fold
			from hist_merged m
		),
		hist_folded as (
			select f.* exclude (pos_fold, neg_fold, zero_count,
					positive_bucket_offset, positive_bucket_counts,
					negative_bucket_offset, negative_bucket_counts),
				f.zero_count + f.pos_fold.folded + f.neg_fold.folded as zero_count,
				f.pos_fold.offset as positive_bucket_offset,
				f.pos_fold.counts as positive_bucket_counts,
				f.neg_fold.offset as negative_bucket_offset,
				f.neg_fold.counts as negative_bucket_counts
			from hist_folds f
		),

		-- What the projection reads. Merging replaces a bucket's datapoints
		-- with one merged datapoint; electing keeps real rows and filters them
		-- by retained_ids; no reduction passes everything through.
		--
		-- `union all by name` matches columns by name rather than position, so
		-- the two branches do not have to agree on column order -- which they
		-- would silently get wrong.
		-- The scalar rows, carrying the per-interval activity computed above.
		--
		-- A left join: the first datapoint of each series has no predecessor and
		-- therefore no delta, and a NaN reading was dropped from scalar_dps
		-- entirely. Both arrive here as null, which is the honest answer -- "no
		-- interval" rather than a zero that would read as "no activity".
		filtered_with_deltas as (
			select d.*, sd.delta as delta, sd.is_reset as is_reset
			from filtered_dps d
			left join scalar_deltas sd
				on sd.series_id = d.series_id and sd.timestamp = d.timestamp
		),
		projected_dps as (
			select * from filtered_with_deltas
			where (select kind from reduction_kind) <> 'merge'
			union all by name
			select
				m.id, m.series_id, m.timestamp, m.start_time,
				m.metric_type, m.aggregation_temporality, m.flags,
				m.count, m.sum,
				-- Bucket-derived, because a merge cannot carry min and max
				-- through: they describe individual observations, and the
				-- merged buckets are what is left of them.
				(bucket_extents(case
					when m.metric_type = 'Histogram'
						then hist_buckets(m.explicit_bounds, m.bucket_counts)
					else exp_buckets(m.scale, m.negative_bucket_offset, m.negative_bucket_counts,
					                 m.zero_count, m.positive_bucket_offset, m.positive_bucket_counts)
				end)).min as min,
				(bucket_extents(case
					when m.metric_type = 'Histogram'
						then hist_buckets(m.explicit_bounds, m.bucket_counts)
					else exp_buckets(m.scale, m.negative_bucket_offset, m.negative_bucket_counts,
					                 m.zero_count, m.positive_bucket_offset, m.positive_bucket_counts)
				end)).max as max,
				m.attribute_ids,
				m.explicit_bounds, m.bucket_counts,
				m.scale, m.zero_count, m.zero_threshold,
				m.positive_bucket_offset, m.positive_bucket_counts,
				m.negative_bucket_offset, m.negative_bucket_counts,
				null::double as double_value, null::bigint as int_value,
				null::varchar as value_type, m.is_monotonic,
				-- Histograms have no scalar to difference.
				null::double as delta, null::boolean as is_reset
			from hist_folded m
			-- A bucket whose datapoints disagree about explicit bounds cannot
			-- be merged; there is no rescale that reconciles two boundary sets.
			-- Dropping the row would hide it, so the merge refuses to run and
			-- the caller sees the unreduced series instead.
			where m.distinct_bounds <= 1
		),

		ts_dps_agg as (
			select
				d.series_id,
				-- The series id is the key, and the only key. It is
				-- content-derived from (stream, instance, labels), so it
				-- distinguishes replicas whose labels are identical, and it is
				-- stable across restarts -- which is what makes it safe in a
				-- URL, unlike a datapoint id that retention eventually deletes.
				--
				-- Deliberately NOT grouped alongside the ingest's resource_id,
				-- as it once was. A resource is content-addressed, so enriching
				-- one mid-stream mints a second resources row for the same
				-- instance, and grouping by it split one series into two chart
				-- lines even after the series id itself stopped splitting. The
				-- resource shown comes from metric_series, which holds exactly
				-- one per series.
				d.series_id::varchar as attrs_key,
				attrs_json(any_value(d.attribute_ids)) as attributes_sample,
				max(d.timestamp) as latest_ts,
				-- Stats over every datapoint in the window, not over whatever
				-- subset a chart ends up drawing.
				--
				-- The client computes these from its chart points, which are
				-- thinned to CHART_POINTS_PER_SERIES before it sees them -- so
				-- the average is the mean of an arbitrary sample, and the total
				-- offered for delta sums is short by roughly the thinning
				-- factor. Both are wrong today and get wronger under any
				-- server-side reduction, which deliberately keeps extremes.
				--
				-- coalesce(double_value, int_value): a datapoint carries one or
				-- the other by metric type, and value_type says which. Both are
				-- null for histogram datapoints, so these come back null there
				-- and the histogram path ignores them -- it has its own totals.
				count(coalesce(d.double_value, d.int_value)) as value_count,
				min(coalesce(d.double_value, d.int_value)) as value_min,
				max(coalesce(d.double_value, d.int_value)) as value_max,
				sum(coalesce(d.double_value, d.int_value)) as value_sum,
				to_json(list(datapoint_json(
					d,
					coalesce((select exemplars from exemplars_agg where exemplars_agg.datapoint_id = d.id), json('[]')),
					(select quantiles from input)
				) order by d.timestamp desc)
					-- Only the reduction narrows the list. The stats above are
					-- deliberately outside this filter: they describe the
					-- window, not the sample drawn from it, which is the whole
					-- reason they are computed here rather than in the client.
					filter (where (select kind from reduction_kind) <> 'elect' or r.id is not null)
				) as datapoints
			-- Grouping on a fixed-width, indexable column instead of rebuilding
		-- and hashing a LIST per row. Measured on 294,607 datapoints: 5.0ms
		-- by the array against 0.9ms by a single uuid.
		from projected_dps d
			left join retained_ids r on r.id = d.id
			group by d.series_id
		),
		-- Pack each timeseries into the wire shape and order them so
		-- the most recently active timeseries sorts first -- mirrors
		-- the "newest first" feel of the old flat datapoint list,
		-- which is what the detail panel's legend reads top-down.
		-- Empty list (no dps in window) collapses to '[]' via the
		-- outer coalesce.
		-- Each series carries the resource that emitted it.
		--
		-- Not optional once series split by resource: two replicas of one
		-- service produce byte-identical attribute sets, so the resource is the
		-- only thing that tells them apart. Without it the legend shows two
		-- entries a user cannot distinguish, which is worse than the single
		-- merged line the split replaced.
		--
		-- The top-level resource (from the representative ingest) stays for
		-- compatibility, but it is the weaker claim: it describes one arbitrary
		-- batch, whereas this describes the line being drawn.
		timeseries_agg as (
			select to_json(list(timeseries_json(
				t.attrs_key,
				t.attributes_sample,
				resource_json(r.attribute_ids, r.dropped_attributes_count),
				t.datapoints,
				series_stats_json(t.value_count, t.value_min, t.value_max, t.value_sum),
				sv.views
			-- attrs_key breaks ties, and the tie is the common case rather
			-- than the exception: series of one metric are usually reported
			-- together, so they share a latest_ts. DuckDB's sort is not
			-- stable, so without a second key the same request returns the
			-- series in a different order each time -- verified by calling
			-- getMetric twice against an unchanged store and getting two
			-- orderings. The UI keys legend rows and colour assignment on
			-- this list, so that reshuffles a chart between refreshes.
			--
			-- attrs_key is the series id: unique within the metric, so the
			-- order is now total and deterministic.
			) order by t.latest_ts desc, t.attrs_key)) as timeseries
			from ts_dps_agg t
			join metric_series ms on ms.id = t.series_id
			join resources r on r.id = ms.resource_id
			-- Left: a histogram series has no scalar views, and a scalar series
			-- with nothing in the window has no buckets either.
			left join scalar_views_agg sv on sv.series_id = t.series_id
		),
		-- The cross-series aggregate: the selected series merged into one
		-- histogram per time bucket, which is what a heatmap draws and what a
		-- window summary describes.
		--
		-- Same five steps as the per-series chain above -- common scale,
		-- aligned offsets, pad, sum, fold -- partitioned by bucket alone
		-- rather than by (series, bucket). It runs on the reduced set, one row
		-- per series per bucket, not on raw datapoints, which is why it costs
		-- ~14ms where the scan that precedes it costs ~140ms.
		--
		-- Temporality is already resolved by hist_folded: Delta summed, and
		-- Cumulative reduced to last-minus-first. Either way each row is
		-- "activity in this bucket", so summing across series is right for
		-- both without branching again.
		--
		-- Histograms only. Summing gauges across series would be arithmetic
		-- nobody asked for.
		agg_scaled as (
			select f.*, min(f.scale) over (partition by f.bucket_start) as agg_scale
			from hist_folded f
		),
		agg_downscaled as (
			select a.*,
				downscale_exp_buckets(a.positive_bucket_counts, a.positive_bucket_offset,
					a.scale - a.agg_scale) as pos_d,
				downscale_exp_buckets(a.negative_bucket_counts, a.negative_bucket_offset,
					a.scale - a.agg_scale) as neg_d
			from agg_scaled a
		),
		agg_aligned as (
			select d.*,
				min(case when len(d.pos_d.counts) > 0 then d.pos_d.offset end)
					over (partition by d.bucket_start) as pos_agg_offset,
				min(case when len(d.neg_d.counts) > 0 then d.neg_d.offset end)
					over (partition by d.bucket_start) as neg_agg_offset
			from agg_downscaled d
		),
		agg_padded as (
			select a.*,
				pad_left_to_offset(a.pos_d.counts, a.pos_d.offset,
					coalesce(a.pos_agg_offset, a.pos_d.offset)) as pos_p,
				pad_left_to_offset(a.neg_d.counts, a.neg_d.offset,
					coalesce(a.neg_agg_offset, a.neg_d.offset)) as neg_p
			from agg_aligned a
		),
		agg_merged as (
			select
				p.bucket_start,
				-- The bucket, for the same reason hist_merged uses it: this row
				-- is a merge across every selected series in one bucket, so no
				-- single constituent's timestamp describes it. Both halves of
				-- the response have to agree on this, or the heatmap's columns
				-- and the per-series rows beneath them sit on different clocks.
				p.bucket_start as timestamp,
				min(p.start_time) as start_time,
				sum(p.count) as count,
				sum(p.sum) as sum,
				any_value(p.agg_scale) as scale,
				max(p.zero_threshold) as zero_threshold,
				sum(p.zero_count) as zero_count,
				any_value(coalesce(p.pos_agg_offset, 0)) as positive_bucket_offset,
				sum_bucket_vectors(list(p.pos_p)) as positive_bucket_counts,
				any_value(coalesce(p.neg_agg_offset, 0)) as negative_bucket_offset,
				sum_bucket_vectors(list(p.neg_p)) as negative_bucket_counts,
				-- Explicit-bounds histograms keep their data here, not in the
				-- exponential vectors above, and merging only those left four of
				-- the five histograms in the reference capture aggregating to
				-- empty buckets with correct totals -- a chart of nothing.
				--
				-- Summed, not diffed: hist_folded already resolved temporality
				-- per series, so every row arriving here is activity within its
				-- bucket whichever way it was recorded.
				sum_bucket_vectors(list(p.bucket_counts)) as bucket_counts,
				any_value(p.explicit_bounds) as explicit_bounds,
				-- Bounds cannot be reconciled the way scales can: there is no
				-- rescale that turns one set of boundaries into another. So a
				-- disagreement is reported rather than merged, the same guard
				-- hist_merged applies along the time axis.
				count(distinct p.explicit_bounds::varchar) as distinct_bounds
			from agg_padded p
			group by p.bucket_start
		),
		-- Same reconciliation the per-series merge needs: taking the largest
		-- zero threshold leaves buckets underneath it that belong in zero_count.
		agg_folds as (
			select m.*,
				zero_fold(m.positive_bucket_counts, m.positive_bucket_offset,
					m.zero_threshold, m.scale) as pos_fold,
				zero_fold(m.negative_bucket_counts, m.negative_bucket_offset,
					m.zero_threshold, m.scale) as neg_fold
			from agg_merged m
		),
		aggregate_agg as (
			select to_json(list(aggregate_bucket_json(
				f.timestamp, f.start_time, f.count, f.sum, f.scale,
				f.zero_threshold, f.zero_count, f.pos_fold, f.neg_fold,
				f.explicit_bounds, f.bucket_counts,
				(select quantiles from input)
			) order by f.bucket_start)) as aggregate
			from agg_folds f
			-- A bucket whose series disagree about bounds cannot be merged.
			-- Dropping it is what the time-axis merge does too; showing a sum
			-- across incompatible layouts would be a plausible chart of nothing.
			where f.distinct_bounds <= 1
		)
		-- Left join: a stream with no datapoints in the window still
		-- produces a row (empty timeseries, blank representative fields).
		-- Only an unknown stream yields zero rows -> sql.ErrNoRows ->
		-- ErrStreamIDNotFound. The representative-sourced fields are
		-- coalesced so the wire shape stays non-null either way.
		select cast(json_object(
			'id', s.id, 'name', s.name, 'description', coalesce(r.description, ''), 'unit', s.unit,
			'metricType', s.metric_type,
			'aggregationTemporality', s.aggregation_temporality,
			'isMonotonic', s.is_monotonic,
			'resourceDroppedAttributesCount', coalesce((select resource_dropped from representative_owners), 0),
			'resource', coalesce(
				(select resource_json(resource_attribute_ids, resource_dropped) from representative_owners),
				json_object('attributes', json('[]'), 'droppedAttributesCount', 0)
			),
			'scopeName', s.scope_name, 'scopeVersion', s.scope_version,
			'scopeDroppedAttributesCount', coalesce((select scope_dropped from representative_owners), 0),
			'scope', coalesce(
				(select scope_json(s.scope_name, s.scope_version, scope_attribute_ids, scope_dropped) from representative_owners),
				json_object('name', s.scope_name, 'version', s.scope_version,
				            'attributes', json('[]'), 'droppedAttributesCount', 0)
			),
			'timeseries', coalesce((select timeseries from timeseries_agg), json('[]')),
			-- Null rather than [] when there is nothing to aggregate, so the
			-- client can tell "no histogram merge happened" from "merged to
			-- nothing".
			'aggregate', (select aggregate from aggregate_agg),
			-- How many datapoints the window actually holds, as opposed to how
			-- many came back. Equal today; the moment the server reduces what it
			-- returns, the difference is what the UI needs in order to say so.
			'datapointCount', coalesce((select sum(dp_count) from (
				select count(*) as dp_count from filtered_dps group by series_id
			)), 0),
			-- The window the reduction actually divided, and whether it came
			-- from the data or from the caller.
			--
			-- Reported rather than left to be inferred: the client drew its axis
			-- by scanning the timestamps it got back, which are bucket starts,
			-- so its axis and the server's bucketing were two answers to one
			-- question -- and the client's was derived from the very reduction
			-- it was trying to describe. Null start/end when the window is the
			-- caller's own: it already knows it.
			'window', json_object(
				'fittedToData', (select fit_to_data from input),
				'startNs', case when (select fit_to_data from input)
					then (select min_ts from data_extent)::varchar end,
				'endNs', case when (select fit_to_data from input)
					then (select max_ts from data_extent)::varchar end
			)
		) as varchar) as metric
		from stream s left join representative r on true
	