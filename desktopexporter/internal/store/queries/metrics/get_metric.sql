
		with input as (
			select ?::uuid as stream_id,
				?::bigint as time_start,
				?::bigint as time_end
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
				) order by d.timestamp desc)) as datapoints
			-- Grouping on a fixed-width, indexable column instead of rebuilding
		-- and hashing a LIST per row. Measured on 294,607 datapoints: 5.0ms
		-- by the array against 0.9ms by a single uuid.
		from filtered_dps d
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
				'datapoints', t.datapoints
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
			'timeseries', coalesce((select timeseries from timeseries_agg), json('[]'))
		) as varchar) as metric
		from stream s left join representative r on true
	