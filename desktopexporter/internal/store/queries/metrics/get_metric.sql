
		with input as (
			select ?::uuid as stream_id,
				?::bigint as time_start,
				?::bigint as time_end,
				?::bigint as target_buckets
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
		-- resource_id joins in so a series can be split by the resource that
		-- emitted it. A join rather than a denormalized column on datapoints:
		-- it is a primary-key lookup from metric_ingest_id, and datapoints is
		-- the largest table in the store.
		filtered_dps as (
			select d.*,
				mi.resource_id as resource_id,
				s.metric_type as metric_type,
				s.aggregation_temporality as aggregation_temporality,
				s.is_monotonic as is_monotonic
			from datapoints d, input, stream s
			join metric_ingests mi on mi.id = d.metric_ingest_id
			where d.stream_id = input.stream_id
			  and d.timestamp >= input.time_start and d.timestamp <= input.time_end
		),
		-- The dp_attrs_agg and exemplar_attrs CTEs are gone: attrs_json
		-- resolves each row's id array in place, so there is nothing to
		-- pre-aggregate and join back.
		exemplars_agg as (
			select e.datapoint_id, json_group_array(json_object(
				'timestamp', e.timestamp::varchar,
				'value', e.value,
				'traceID', trace_id_wire(e.trace_id),
				'spanID', span_id_wire(e.span_id),
				'filteredAttributes', attrs_json(e.attribute_ids)
			)) as exemplars
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
		-- resource, attribute_ids), so the array cannot vary inside a group.
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
					then bucket_width_ns(i.time_end - i.time_start, i.target_buckets)
				-- Histograms reduce by merging. Delta adds bucket counts;
				-- Cumulative subtracts the first of the bucket from the last,
				-- because each datapoint is a running total and adding them
				-- would count every observation once per datapoint.
				when s.metric_type in ('Histogram', 'ExponentialHistogram')
				     and s.aggregation_temporality in ('Delta', 'Cumulative')
					then bucket_width_ns(i.time_end - i.time_start, i.target_buckets)
			end as width_ns
			from input i, stream s
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
		bucketed_dps as (
			select d.*,
				(d.timestamp // (select width_ns from reduction))
					* (select width_ns from reduction) as bucket_start
			from filtered_dps d
			where (select width_ns from reduction) is not null
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
				p.resource_id,
				p.bucket_start,
				-- A real datapoint id, not a synthetic one: the last of the
				-- bucket. Keeps ?dp= links and datapoint selection working
				-- against something that exists.
				arg_max(p.id, p.timestamp) as id,
				max(p.timestamp) as timestamp,
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
			group by p.series_id, p.resource_id, p.bucket_start
		),

		-- What the projection reads. Merging replaces a bucket's datapoints
		-- with one merged datapoint; electing keeps real rows and filters them
		-- by retained_ids; no reduction passes everything through.
		--
		-- `union all by name` matches columns by name rather than position, so
		-- the two branches do not have to agree on column order -- which they
		-- would silently get wrong.
		projected_dps as (
			select * from filtered_dps
			where (select kind from reduction_kind) <> 'merge'
			union all by name
			select
				m.id, m.series_id, m.resource_id, m.timestamp, m.start_time,
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
				m.explicit_bounds, m.bucket_counts,
				m.scale, m.zero_count, m.zero_threshold,
				m.positive_bucket_offset, m.positive_bucket_counts,
				m.negative_bucket_offset, m.negative_bucket_counts,
				null::double as double_value, null::bigint as int_value,
				null::varchar as value_type, m.is_monotonic
			from hist_merged m
			-- A bucket whose datapoints disagree about explicit bounds cannot
			-- be merged; there is no rescale that reconciles two boundary sets.
			-- Dropping the row would hide it, so the merge refuses to run and
			-- the caller sees the unreduced series instead.
			where m.distinct_bounds <= 1
		),

		ts_dps_agg as (
			select
				d.series_id,
				d.resource_id,
				-- The series id is the key. It is content-derived from
				-- (stream, resource, labels), so it distinguishes replicas
				-- whose labels are identical, and it is stable across restarts
				-- -- which is what makes it safe in a URL, unlike a datapoint
				-- id that retention eventually deletes.
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
				to_json(list(json_merge_patch(
					json_object(
						'id', d.id,
						'metricType', d.metric_type,
						'timestamp', d.timestamp::varchar,
						'startTime', d.start_time::varchar,
						'flags', d.flags,
						'exemplars', coalesce((select exemplars from exemplars_agg where exemplars_agg.datapoint_id = d.id), json('[]'))
					),
					case d.metric_type
						when 'Gauge' then json_object(
							'doubleValue', d.double_value,
							'intValue', d.int_value,
							'valueType', d.value_type
						)
						when 'Sum' then json_object(
							'doubleValue', d.double_value,
							'intValue', d.int_value,
							'valueType', d.value_type,
							'isMonotonic', d.is_monotonic,
							'aggregationTemporality', d.aggregation_temporality
						)
						when 'Histogram' then json_object(
							'count', d.count,
							'sum', d.sum,
							'min', d.min,
							'max', d.max,
							'bucketCounts', d.bucket_counts,
							'explicitBounds', d.explicit_bounds,
							'aggregationTemporality', d.aggregation_temporality
						)
						when 'ExponentialHistogram' then json_object(
							'count', d.count,
							'sum', d.sum,
							'min', d.min,
							'max', d.max,
							'scale', d.scale,
							'zeroCount', d.zero_count,
							'zeroThreshold', d.zero_threshold,
							'positiveBucketOffset', d.positive_bucket_offset,
							'positiveBucketCounts', d.positive_bucket_counts,
							'negativeBucketOffset', d.negative_bucket_offset,
							'negativeBucketCounts', d.negative_bucket_counts,
							'aggregationTemporality', d.aggregation_temporality
						)
					end
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
			group by d.series_id, d.resource_id
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
			select to_json(list(json_object(
				'attributesKey', t.attrs_key,
				'attributes', t.attributes_sample,
				'resource', resource_json(r.attribute_ids, r.dropped_attributes_count),
				'datapoints', t.datapoints,
				-- Null for histogram series, which have no scalar value.
				'stats', case when t.value_count > 0 then json_object(
					'count', t.value_count,
					'min', t.value_min,
					'max', t.value_max,
					'sum', t.value_sum,
					'avg', t.value_sum / t.value_count
				) end
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
			join resources r on r.id = t.resource_id
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
			-- How many datapoints the window actually holds, as opposed to how
			-- many came back. Equal today; the moment the server reduces what it
			-- returns, the difference is what the UI needs in order to say so.
			'datapointCount', coalesce((select sum(dp_count) from (
				select count(*) as dp_count from filtered_dps group by series_id
			)), 0)
		) as varchar) as metric
		from stream s left join representative r on true
	