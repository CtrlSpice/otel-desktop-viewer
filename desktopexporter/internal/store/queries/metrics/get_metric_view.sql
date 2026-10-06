
		with input as (
			select ?::uuid as metric_id,
				list_extract(?::ubigint[], 1) as time_start,
				list_extract(?::ubigint[], 1) as time_end,
				?::bigint as target_buckets,
				-- NULL means all series; [] means none. Bind as text because the
				-- driver corrupts bound []duckdb.UUID values.
				?::varchar[] as series_ids,
				-- Histogram quantiles; [] skips quantile computation.
				?::double[] as quantiles,
				-- Viewer UTC offset in nanoseconds; 0 aligns buckets to UTC.
				?::bigint as tz_offset_ns,
				-- Sum, Average, and Rate resolution, independent of target_buckets.
				?::bigint as view_buckets,
				-- Sparkline resolution; extrema emit at most two points per bucket.
				?::bigint as sparkline_buckets,
				-- Series in the Selected aggregate; NULL means none selected.
				?::varchar[] as selected_series_ids,
				-- IANA zone for per-bucket offsets; NULL uses tz_offset_ns.
				?::varchar as tz_name,
				?::varchar[] as datapoint_series_ids,
				-- Datapoint-series limit in response order; 0 means unlimited and
				-- datapoint_series_ids takes precedence.
				?::bigint as datapoint_series_limit
		),
		selected_metric as (
			select s.*, r.schema_url as resource_schema_url,
				r.attribute_ids as resource_attribute_ids,
				r.dropped_attributes_count as resource_dropped_attributes_count,
				sc.name as scope_name, sc.version as scope_version,
				sc.attribute_ids as scope_attribute_ids,
				sc.schema_url as scope_schema_url,
				sc.dropped_attributes_count as scope_dropped_attributes_count
			from metrics s
			join resources r on r.id = s.resource_id
			join scopes sc on sc.id = s.scope_id, input
			where s.id = input.metric_id
		),
		-- Attach parent Metric descriptor fields to datapoints.
		filtered_dps as (
			-- Resolve the shared bounds vector once at the datapoint boundary.
			select d.* exclude (bounds_id),
				hb.bounds as explicit_bounds,
				-- Per-instant zone offset, falling back to the fixed caller offset.
				coalesce(tz_offset_ns_at(d.timestamp, input.tz_name),
				         input.tz_offset_ns) as tz_shift,
				s.metric_type as metric_type,
				s.aggregation_temporality as aggregation_temporality,
				s.is_monotonic as is_monotonic
			from metric_datapoints d
			left join histogram_bounds hb on hb.id = d.bounds_id,
				input, selected_metric s
			where d.metric_id = input.metric_id
			  {{.TimeFilter}}
			  -- Filter before reduction. NULL means no filter; [] means no series.
			  and (input.series_ids is null
			       or list_contains(input.series_ids, d.series_id::varchar))
		),
		-- Rank exemplars from both value extremes. Non-finite and absent values
		-- sort last. DOUBLE orders mixed numeric values; HUGEINT distinguishes
		-- adjacent int64 values that DOUBLE cannot represent separately.
		exemplars_valued as (
			select e.*,
				case
					when e.int_value is not null then struct_pack(
						approx := e.int_value::double,
						exact_int := e.int_value::hugeint
					)
					when isfinite(e.double_value) then struct_pack(
						approx := e.double_value,
						exact_int := try_cast(e.double_value as hugeint)
					)
				end as value_order
			from exemplars e
			where e.metric_datapoint_id in (select id from filtered_dps)
		),
		exemplars_ranked as (
			select e.*,
				row_number() over (partition by e.metric_datapoint_id
				                   order by e.from_end, e.id) as rn
			from (
				select e.*,
					least(
						row_number() over (partition by e.metric_datapoint_id
							order by e.value_order asc nulls last, e.id),
						row_number() over (partition by e.metric_datapoint_id
							order by e.value_order desc nulls last, e.id)
					) as from_end
				from exemplars_valued e
			) e
		),

		-- Value extents used to rank exemplar-bearing datapoints.
		exemplar_extents as (
			select metric_datapoint_id,
				min(value_order) as low,
				max(value_order) as high
			from exemplars_ranked
			group by metric_datapoint_id
		),
		exemplars_agg as (
			-- (timestamp, id) gives total order; json_group_array rejects ORDER BY.
			select e.metric_datapoint_id,
				-- Retain at most five exemplars per datapoint.
				to_json(list(exemplar_json(e) order by e.timestamp, e.id)
				        filter (where e.rn <= 5)) as exemplars,
				-- Count before capping so the wire reports withheld exemplars.
				count(*) as exemplar_count
			from exemplars_ranked e
			group by e.metric_datapoint_id
		),
		-- M4 retains each bucket's earliest, latest, minimum, and maximum scalar
		-- datapoints. NULL target_buckets disables reduction. The inclusive span
		-- adds 1ns so one datapoint still occupies a non-zero window.
		data_extent as (
			select min(timestamp) as min_ts, max(timestamp) as max_ts
			from filtered_dps
		),
		effective_window as (
			select coalesce(i.time_start, e.min_ts) as start_ns,
				coalesce(i.time_end, e.max_ts) as end_ns
			from input i, data_extent e
		),
		reduction_span as (
			select case
				when start_ns is not null and end_ns is not null then least(
					end_ns::hugeint - start_ns::hugeint + 1,
					9223372036854775807::hugeint)::bigint
			end as span_ns
			from effective_window
		),
		reduction as (
			select case
				-- Gauge and Sum elect scalar points; histograms merge counts.
				when s.metric_type in ('Gauge', 'Sum')
					then bucket_width_ns(rs.span_ns, i.target_buckets)
				-- Delta histograms add activity; Cumulative histograms difference
				-- consecutive running totals before adding.
				when s.metric_type in ('Histogram', 'ExponentialHistogram')
				     and s.aggregation_temporality in (1, 2)
					then bucket_width_ns(rs.span_ns, i.target_buckets)
			end as width_ns
			-- Keep lambda arguments relational; DuckDB rejects subqueries in lambdas.
			from input i, selected_metric s, reduction_span rs
		),

		-- Histogram merge and scalar election share bucket boundaries.
		reduction_kind as (
			select case
				when (select width_ns from reduction) is null then 'none'
				when s.metric_type in ('Histogram', 'ExponentialHistogram') then 'merge'
				else 'elect'
			end as kind
			from selected_metric s
		),

		-- Shift to local time, floor on the absolute grid, then shift to UTC.
		-- floor_div handles negative pre-epoch timestamps correctly.
		bucketed_dps as (
			select d.*,
				case
					-- A one-bucket summary covers the whole window rather than one
					-- absolute ladder interval.
					when (select target_buckets from input) = 1
						then coalesce((select min_ts from data_extent), d.timestamp)
					else
						bucket_start_utc(
							floor_div(d.timestamp::hugeint + d.tz_shift::hugeint,
							          (select width_ns from reduction))
								::hugeint * (select width_ns from reduction)::hugeint,
							(select tz_name from input),
							(select tz_offset_ns from input))
				end as bucket_start
			from filtered_dps d
			where (select width_ns from reduction) is not null
		),

		-- Difference cumulative values before reduction. The first reading has no
		-- interval; after a reset, the current value is the interval activity.
		-- Non-finite doubles remain in the datapoint response but not arithmetic.
		scalar_sequence as (
			select d.series_id, d.id, d.timestamp, d.tz_shift,
				d.metric_type, d.is_monotonic,
				d.double_value, d.int_value,
				coalesce(d.double_value, d.int_value) as value,
				case
					when d.int_value is not null then d.int_value::hugeint
					when trunc(d.double_value) = d.double_value
						then try_cast(d.double_value as hugeint)
				end as exact_integer
			from filtered_dps d
			where d.metric_type in ('Gauge', 'Sum')
			  -- Empty arms break intervals; non-finite doubles skip arithmetic.
			  and (d.int_value is not null or d.double_value is null
			       or isfinite(d.double_value))
		),
		-- Views and sparklines use numeric observations only.
		scalar_dps as (
			select * from scalar_sequence
			where int_value is not null or double_value is not null
		),
		scalar_lagged as (
			select s.*,
				lag(s.id) over (
					partition by s.series_id order by s.timestamp, s.id
				) as prev_id,
				lag(s.double_value) over (
					partition by s.series_id order by s.timestamp, s.id
				) as prev_double_value,
				lag(s.int_value) over (
					partition by s.series_id order by s.timestamp, s.id
				) as prev_int_value,
				lag(s.exact_integer) over (
					partition by s.series_id order by s.timestamp, s.id
				) as prev_exact_integer
			from scalar_sequence s
		),
		scalar_compared as (
			select l.*,
				case when exact_integer is not null and prev_exact_integer is not null then case
					when prev_exact_integer < 0 then
						exact_integer <= 170141183460469231731687303715884105727::hugeint
							+ prev_exact_integer
					when prev_exact_integer > 0 then
						exact_integer >= (-170141183460469231731687303715884105727::hugeint - 1)
							+ prev_exact_integer
					else true
				end
				end as exact_difference_fits,
				case
					when (int_value is null and double_value is null)
					  or (prev_int_value is null and prev_double_value is null) then null
					when metric_type = 'Sum' and is_monotonic then case
					when int_value is not null and prev_int_value is not null
						then int_value < prev_int_value
					when double_value is not null and prev_double_value is not null
						then double_value < prev_double_value
					when int_value is not null then case
						when prev_double_value >= 9223372036854775808.0 then true
						when prev_double_value <= -9223372036854775808.0 then false
						else int_value::hugeint < ceil(prev_double_value)::hugeint
					end
					else case
						when double_value < -9223372036854775808.0 then true
						when double_value >= 9223372036854775808.0 then false
						else floor(double_value)::hugeint < prev_int_value::hugeint
					end
					end
					else false
				end as is_reset
			from scalar_lagged l
			where prev_id is not null
		),
		-- Join deltas by datapoint ID because OTLP permits duplicate timestamps.
		scalar_deltas as (
			select series_id,
				id,
				timestamp,
				case
					when exact_integer is not null and prev_exact_integer is not null
					 and (int_value is not null or prev_int_value is not null)
					 and (is_reset or exact_difference_fits)
						then case when is_reset then exact_integer
						          else exact_integer - prev_exact_integer end
					when exact_integer is not null and is_reset
					 and (int_value is not null or prev_int_value is not null)
						then exact_integer
				end as delta_int,
				case
					when exact_integer is not null and prev_exact_integer is not null
					 and (int_value is not null or prev_int_value is not null)
					 and (is_reset or exact_difference_fits) then null
					when exact_integer is not null and is_reset
					 and (int_value is not null or prev_int_value is not null) then null
					when is_reset then coalesce(double_value, int_value::double)
					else coalesce(double_value, int_value::double)
						- coalesce(prev_double_value, prev_int_value::double)
				end as delta_double,
				is_reset
			from scalar_compared
		),

		-- Use a shared absolute grid no finer than the Metric's median reporting
		-- cadence. Per-series medians ignore outages; the median across series
		-- prevents one series setting the grid. Legend changes do not alter it.
		series_gaps as (
			select d.series_id,
				d.timestamp::hugeint - lag(d.timestamp) over (
					partition by d.series_id order by d.timestamp, d.id
				)::hugeint as gap_ns
			from scalar_dps d
		),
		series_cadence as (
			select median(gap_ns) as cadence_ns
			from (
				select series_id, median(gap_ns) as gap_ns
				from series_gaps
				where gap_ns is not null and gap_ns > 0
				group by series_id
			)
		),
		-- Keep bucket_width_ns arguments relational; DuckDB rejects subqueries
		-- in lambda expressions.
		scalar_view_grid as (
			-- Cap bucket count by reporting intervals, then select a ladder rung.
			-- A requested count of 0 must still produce NULL width.
			select bucket_width_ns(rs.span_ns,
				case when c.cadence_ns is null or c.cadence_ns <= 0
					then i.view_buckets
					else least(i.view_buckets,
					           greatest(1, (rs.span_ns // c.cadence_ns)::bigint))
				end) as width_ns
			from input i, reduction_span rs, series_cadence c
		),
		scalar_view_bucketed as (
			select d.series_id,
				floor_div(d.timestamp::hugeint + d.tz_shift::hugeint,
				          (select width_ns from scalar_view_grid))
					::hugeint * (select width_ns from scalar_view_grid)::hugeint as bucket_local,
				bucket_start_utc(
					floor_div(d.timestamp::hugeint + d.tz_shift::hugeint,
					          (select width_ns from scalar_view_grid))
						::hugeint * (select width_ns from scalar_view_grid)::hugeint,
					(select tz_name from input),
					(select tz_offset_ns from input)) as bucket_start,
				d.value,
				coalesce(sd.delta_double, sd.delta_int::double) as delta,
				sd.is_reset
			from scalar_dps d
			left join scalar_deltas sd
				on sd.id = d.id
		),
		-- Trim leading and trailing empty buckets per series on the local grid.
		scalar_view_extent as (
			select series_id,
				min(bucket_local) as first_bucket,
				max(bucket_local) as last_bucket
			from scalar_view_bucketed
			group by series_id
		),
		-- Preserve interior gaps. Step in local time, convert each bucket to UTC,
		-- and deduplicate skipped DST instants that map to one bucket.
		scalar_view_spine as (
			select distinct series_id, bucket_start
			from (
				select e.series_id,
					bucket_start_utc(
						e.first_bucket + unnest(range(
							((e.last_bucket - e.first_bucket) // g.width_ns)::bigint + 1
						))::hugeint * g.width_ns::hugeint,
						(select tz_name from input),
						(select tz_offset_ns from input)) as bucket_start
				from scalar_view_extent e, scalar_view_grid g
			)
		),
		-- One row per (series, bucket). Empty Sum/Rate buckets draw zero; Average
		-- remains NULL. Only Rate uses cumulative differences.
		scalar_view_agg as (
			select sp.series_id,
				sp.bucket_start,
				count(b.series_id) as sample_count,
				sum(b.value) as value_sum,
				avg(b.value) as value_avg,
				sum(b.delta) / ((select width_ns from scalar_view_grid) / 1e9) as rate,
				-- Mark any cumulative-counter reset within the bucket.
				coalesce(bool_or(b.is_reset), false) as has_reset
			from scalar_view_spine sp
			left join scalar_view_bucketed b
				on b.series_id = sp.series_id
			   and b.bucket_start = sp.bucket_start
			group by sp.series_id, sp.bucket_start
		),
		-- Empty buckets draw zero; a first sampled bucket without rate is omitted.
		scalar_view_drawn as (
			select series_id, bucket_start,
				case when sample_count = 0 then 0 else rate end as drawn_rate
			from scalar_view_agg
			where sample_count = 0 or rate is not null
		),
		-- Incoming segment slope = delta rate / elapsed seconds.
		scalar_view_slope as (
			select series_id, bucket_start,
				(drawn_rate - lag(drawn_rate) over w)
					/ ((bucket_start - lag(bucket_start) over w) / 1e9) as slope
			from scalar_view_drawn
			window w as (partition by series_id order by bucket_start)
		),
		-- Rate stats include drawn gap zeros.
		scalar_rate_stats as (
			select series_id,
				json_object('min', double_wire_json(min(drawn_rate)),
				            'max', double_wire_json(max(drawn_rate)),
				            'avg', double_wire_json(avg(drawn_rate))) as rate_stats
			from scalar_view_drawn
			group by series_id
		),
		scalar_views_agg as (
			select a.series_id,
				to_json(list(json_object(
					'bucketStart', a.bucket_start::varchar,
					'sampleCount', a.sample_count,
					'sum', double_wire_json(a.value_sum),
					'avg', double_wire_json(a.value_avg),
					'rate', double_wire_json(a.rate),
					'slope', double_wire_json(sl.slope),
					'hasReset', a.has_reset
				) order by a.bucket_start)) as views
			from scalar_view_agg a
			left join scalar_view_slope sl
				on sl.series_id = a.series_id and sl.bucket_start = a.bucket_start
			group by a.series_id
		),

		-- Fold all-series and selected-series pools on the per-series grid.
		-- Sum adds values, Average weights every sample, and Rate adds rates.
		scalar_pool_rows as (
			select 'all' as pool, a.* from scalar_view_agg a
			union all by name
			select 'selected' as pool, a.*
			from scalar_view_agg a, input i
			where i.selected_series_ids is not null
			  and list_contains(i.selected_series_ids, a.series_id::varchar)
		),
		-- Inherit per-series spines so interior gaps remain and absent pool extents
		-- do not become invented zero observations.
		scalar_pool_agg as (
			select pool,
				bucket_start,
				sum(sample_count) as sample_count,
				sum(value_sum) as value_sum,
				sum(value_sum) / nullif(sum(sample_count), 0) as value_avg,
				sum(rate) as rate,
				coalesce(bool_or(has_reset), false) as has_reset
			from scalar_pool_rows
			group by pool, bucket_start
		),
		-- Match the per-series bucket shape.
		scalar_pool_drawn as (
			select pool, bucket_start,
				case when sample_count = 0 then 0 else rate end as drawn_rate
			from scalar_pool_agg
			where sample_count = 0 or rate is not null
		),
		scalar_pool_slope as (
			select pool, bucket_start,
				(drawn_rate - lag(drawn_rate) over w)
					/ ((bucket_start - lag(bucket_start) over w) / 1e9) as slope
			from scalar_pool_drawn
			window w as (partition by pool order by bucket_start)
		),
		scalar_pools_json as (
			select
				coalesce(to_json(list(json_object(
					'bucketStart', a.bucket_start::varchar,
					'sampleCount', a.sample_count,
					'sum', double_wire_json(a.value_sum),
					'avg', double_wire_json(a.value_avg),
					'rate', double_wire_json(a.rate),
					'slope', double_wire_json(sl.slope),
					'hasReset', a.has_reset
				) order by a.bucket_start) filter (where a.pool = 'selected')), json('[]')) as selected,
				coalesce(to_json(list(json_object(
					'bucketStart', a.bucket_start::varchar,
					'sampleCount', a.sample_count,
					'sum', double_wire_json(a.value_sum),
					'avg', double_wire_json(a.value_avg),
					'rate', double_wire_json(a.rate),
					'slope', double_wire_json(sl.slope),
					'hasReset', a.has_reset
				) order by a.bucket_start) filter (where a.pool = 'all')), json('[]')) as all_series
			from scalar_pool_agg a
			left join scalar_pool_slope sl
				on sl.pool = a.pool and sl.bucket_start = a.bucket_start
		),

		-- Sparkline reduction keeps min/max shape without an empty-bucket spine.
		sparkline_grid as (
			-- Do not bucket more finely than the Metric's cadence.
			select bucket_width_ns(rs.span_ns,
				case when c.cadence_ns is null or c.cadence_ns <= 0
					then i.sparkline_buckets
					else least(i.sparkline_buckets,
					           greatest(1, (rs.span_ns // c.cadence_ns)::bigint))
				end) as width_ns
			from input i, reduction_span rs, series_cadence c
		),
		sparkline_bucketed as (
			select d.series_id,
				bucket_start_utc(
					floor_div(d.timestamp::hugeint + d.tz_shift::hugeint,
					          (select width_ns from sparkline_grid))
						::hugeint * (select width_ns from sparkline_grid)::hugeint,
					(select tz_name from input),
					(select tz_offset_ns from input)) as bucket_start,
				d.timestamp,
				d.value
			from scalar_dps d
			-- NULL width disables sparklines rather than forming one NULL group.
			where (select width_ns from sparkline_grid) is not null
		),
		-- Keep each extremum's actual timestamp.
		sparkline_extrema as (
			select series_id,
				bucket_start,
				-- Timestamp tie-breaks keep flat buckets deterministic.
				arg_min(timestamp, (value, timestamp)) as min_ts,
				min(value) as min_value,
				arg_max(timestamp, (value, timestamp)) as max_ts,
				max(value) as max_value
			from sparkline_bucketed
			group by series_id, bucket_start
		),
		-- UNION removes duplicate min/max points from flat buckets.
		sparkline_points as (
			select series_id, min_ts as timestamp, min_value as value
			from sparkline_extrema
			union by name
			select series_id, max_ts as timestamp, max_value as value
			from sparkline_extrema
		),
		-- Histogram series have NULL sparklines.
		sparkline_agg as (
			select series_id,
				to_json(list(json_object(
					'timestamp', timestamp::varchar,
					'value', double_wire_json(value)
				) order by timestamp, value)) as sparkline
			from sparkline_points
			group by series_id
		),

		-- Exclude non-finite values from scalar min/max election.
		bucket_elected as (
			select
				series_id,
				bucket_start,
				-- ID tie-breaks make first/last/min/max election deterministic.
				arg_min(id, (timestamp, id)) as first_id,
				arg_max(id, (timestamp, id)) as last_id,
				arg_min(id, (coalesce(double_value, int_value), id))
					filter (where isfinite(coalesce(double_value, int_value))) as min_id,
				arg_max(id, (coalesce(double_value, int_value), id))
					filter (where isfinite(coalesce(double_value, int_value))) as max_id
			from bucketed_dps
			group by series_id, bucket_start
		),

		-- Retain exemplar-bearing datapoints from both value extremes so scalar
		-- election does not remove all trace links.
		exemplar_carriers as (
			select id from (
				select id,
					row_number() over (partition by series_id, bucket_start
					                   order by from_end, id) as rn
				from (
					select b.id, b.series_id, b.bucket_start,
						least(
							row_number() over (partition by b.series_id, b.bucket_start
								order by x.low asc nulls last, b.id),
							row_number() over (partition by b.series_id, b.bucket_start
								order by x.high desc nulls last, b.id)
						) as from_end
					from bucketed_dps b
					join exemplar_extents x on x.metric_datapoint_id = b.id
				)
			)
			-- At most two extra carriers plus the four elected datapoints.
			where rn <= 2
		),

		-- Retain elected scalar points and capped exemplar carriers.
		retained_ids as (
			select unnest([first_id, last_id, min_id, max_id]) as id from bucket_elected
			union
			select id from exemplar_carriers
		),

		-- Merge histogram activity exactly. Exponential buckets first downscale
		-- to the coarsest scale and left-pad to the smallest offset.
		hist_scaled as (
			select b.*,
				-- Cumulative series share one scale because differences cross buckets;
				-- Delta series align within each bucket.
				case when b.aggregation_temporality = 2
					then min(b.scale) over (partition by b.series_id)
					else min(b.scale) over (partition by b.series_id, b.bucket_start)
				end as target_scale
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
		-- Empty arrays do not influence alignment offsets.
		hist_aligned as (
			select d.*,
				case when d.aggregation_temporality = 2
					then min(case when len(d.pos_d.counts) > 0 then d.pos_d.offset end)
						over (partition by d.series_id)
					else min(case when len(d.pos_d.counts) > 0 then d.pos_d.offset end)
						over (partition by d.series_id, d.bucket_start)
				end as pos_target_offset,
				case when d.aggregation_temporality = 2
					then min(case when len(d.neg_d.counts) > 0 then d.neg_d.offset end)
						over (partition by d.series_id)
					else min(case when len(d.neg_d.counts) > 0 then d.neg_d.offset end)
						over (partition by d.series_id, d.bucket_start)
				end as neg_target_offset
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
		-- Previous cumulative reading for interval activity.
		hist_lagged as (
			select p.*,
				lag(p.count) over w as prev_count,
				lag(p.sum) over w as prev_sum,
				lag(p.zero_count) over w as prev_zero_count,
				lag(p.bucket_counts) over w as prev_bucket_counts,
				lag(p.pos_p) over w as prev_pos_p,
				lag(p.neg_p) over w as prev_neg_p
			from hist_padded p
			window w as (partition by p.series_id order by p.timestamp, p.id)
		),
		-- Convert Cumulative readings to interval activity. A reset uses the later
		-- row for every field; the first reading has no interval and is dropped.
		hist_activity as (
			select l.* exclude (
					count, sum, zero_count, bucket_counts, pos_p, neg_p,
					prev_count, prev_sum, prev_zero_count,
					prev_bucket_counts, prev_pos_p, prev_neg_p
				),
				case when l.aggregation_temporality <> 2 then l.count
					when l.count < l.prev_count then l.count
					else l.count - l.prev_count end as count,
				case when l.aggregation_temporality <> 2 then l.sum
					when l.count < l.prev_count then l.sum
					else l.sum - l.prev_sum end as sum,
				case when l.aggregation_temporality <> 2 then l.zero_count
					when l.count < l.prev_count then l.zero_count
					else l.zero_count - l.prev_zero_count end as zero_count,
				case when l.aggregation_temporality <> 2 then l.bucket_counts
					else coalesce(
						diff_bucket_vectors(l.bucket_counts, l.prev_bucket_counts),
						l.bucket_counts) end as bucket_counts,
				case when l.aggregation_temporality <> 2 then l.pos_p
					else coalesce(
						diff_bucket_vectors(l.pos_p, l.prev_pos_p),
						l.pos_p) end as pos_p,
				case when l.aggregation_temporality <> 2 then l.neg_p
					else coalesce(
						diff_bucket_vectors(l.neg_p, l.prev_neg_p),
						l.neg_p) end as neg_p
			from hist_lagged l
			where l.aggregation_temporality <> 2
			   or l.prev_count is not null
		),

		hist_merged as (
			select
				p.series_id,
				p.bucket_start,
				-- Use the latest real datapoint ID for selection links.
				arg_max(p.id, (p.timestamp, p.id)) as id,
				-- series_id guarantees one attribute_ids value per group.
				any_value(p.attribute_ids) as attribute_ids,
				-- timestamp is the bucket start; start_time is the earliest received
				-- observation-period start among constituents.
				p.bucket_start as timestamp,
				min(p.start_time) as start_time,
				any_value(p.metric_type) as metric_type,
				any_value(p.aggregation_temporality) as aggregation_temporality,
				any_value(p.flags) as flags,
				any_value(p.is_monotonic) as is_monotonic,
				-- hist_activity has already normalized both temporalities to activity.
				sum(p.count) as count,
				case when count(p.sum) = count(*) then sum(p.sum) end as sum,
				-- Explicit bounds must match exactly within the group.
				any_value(p.explicit_bounds) as explicit_bounds,
				count(distinct p.explicit_bounds::varchar) as distinct_bounds,
				sum_bucket_vectors(list(p.bucket_counts)) as bucket_counts,
				any_value(p.target_scale) as scale,
				max(p.zero_threshold) as zero_threshold,
				sum(p.zero_count) as zero_count,
				any_value(coalesce(p.pos_target_offset, 0)) as positive_bucket_offset,
				sum_bucket_vectors(list(p.pos_p)) as positive_bucket_counts,
				any_value(coalesce(p.neg_target_offset, 0)) as negative_bucket_offset,
				sum_bucket_vectors(list(p.neg_p)) as negative_bucket_counts
			from hist_activity p
			group by p.series_id, p.bucket_start
		),

		-- Fold buckets below the largest merged zero threshold into zero_count.
		-- Separate CTEs compute each fold once before unpacking its fields.
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

		-- Add scalar interval activity. First and non-finite readings have NULL delta.
		filtered_with_deltas as (
			select d.*, sd.delta_int, sd.delta_double, sd.is_reset as is_reset
			from filtered_dps d
			left join scalar_deltas sd
				on sd.id = d.id
		),
		projected_dps as (
			select * from filtered_with_deltas
			where (select kind from reduction_kind) <> 'merge'
			union all by name
			select
				m.id, m.series_id, m.timestamp, m.start_time,
				m.metric_type, m.aggregation_temporality, m.flags,
				m.count, m.sum,
				-- Derive extents from merged buckets, not received observations.
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
				-- Histograms have no scalar delta.
				null::hugeint as delta_int, null::double as delta_double,
				null::boolean as is_reset
			from hist_folded m
			-- Drop and report buckets with incompatible explicit bounds.
			where m.distinct_bounds <= 1
		),

		-- Response series order: latest activity first, then series ID.
		datapoint_series_rank as (
			-- Rank projected timestamps so reduced histogram ordering matches the wire.
			select series_id,
				row_number() over (
					order by max(timestamp) desc, series_id::varchar
				) as rn
			from projected_dps
			group by series_id
		),
		-- Named series override the limit; otherwise 0 means all series.
		datapoint_series_allowed as (
			select r.series_id
			from datapoint_series_rank r, input i
			where case
				when i.datapoint_series_ids is not null
					then list_contains(i.datapoint_series_ids, r.series_id::varchar)
				when i.datapoint_series_limit > 0
					then r.rn <= i.datapoint_series_limit
				else true
			end
		),

		-- Full-window count and latest timestamp before reduction and narrowing.
		series_window_counts as (
			select series_id,
				count(*) as datapoint_count,
				max(timestamp) as latest_ns
			from filtered_dps
			group by series_id
		),
		ts_dps_agg as (
			select
				d.series_id,
				-- Stable database-local series reference for URLs and grouping.
				d.series_id::varchar as attrs_key,
				attrs_json(any_value(d.attribute_ids)) as attributes_sample,
				max(d.timestamp) as latest_ts,
				-- Scalar stats cover the full window. Histogram scalar values are NULL.
				count(coalesce(d.double_value, d.int_value)) as value_count,
				min(coalesce(d.double_value, d.int_value)) as value_min,
				max(coalesce(d.double_value, d.int_value)) as value_max,
				sum(coalesce(d.double_value, d.int_value)) as value_sum
			-- Group on the fixed-width indexed series ID.
		from projected_dps d
			group by d.series_id
		),

		-- Build wire JSON only for datapoints surviving reduction and narrowing.
		shipped_dps as (
			select d.* from projected_dps d
			left join retained_ids r on r.id = d.id
			where ((select kind from reduction_kind) <> 'elect' or r.id is not null)
			  and d.series_id in (select series_id from datapoint_series_allowed)
		),
		-- Compute requested quantiles relationally with one bucket walk per datapoint.
		dp_qsrc as (
			select d.id,
				case when d.metric_type = 'Histogram'
					then hist_buckets(d.explicit_bounds, d.bucket_counts)
					else exp_buckets(d.scale,
						d.negative_bucket_offset, d.negative_bucket_counts,
						d.zero_count,
						d.positive_bucket_offset, d.positive_bucket_counts) end as buckets,
				d.metric_type = 'Histogram' as is_linear
			from shipped_dps d
			where d.metric_type in ('Histogram', 'ExponentialHistogram')
			  and len((select quantiles from input)) > 0
		),
		-- One unnest and cumulative count per datapoint.
		dp_q_acc as (
			select b.id, b.is_linear, u.b.lo as lo, u.b.hi as hi, u.b.cnt as cnt, u.i as i,
				coalesce(sum(u.b.cnt) over (partition by b.id order by u.i
					rows between unbounded preceding and 1 preceding), 0) as acc_prev,
				sum(u.b.cnt) over (partition by b.id order by u.i
					rows between unbounded preceding and current row) as acc,
				sum(u.b.cnt) over (partition by b.id) as n
			from dp_qsrc b, unnest(b.buckets) with ordinality u(b, i)
		),
		-- First non-empty bucket whose cumulative count crosses q * n.
		dp_q_picked as (
			select a.id, qq.q,
				case when a.is_linear
					then interp_linear(a.lo, a.hi, a.acc_prev, a.cnt, qq.q * a.n)
					else interp_loglin(a.lo, a.hi, a.acc_prev, a.cnt, qq.q * a.n) end as v,
				row_number() over (partition by a.id, qq.q order by a.i) as rn
			from dp_q_acc a, (select unnest((select quantiles from input)) as q) qq
			where a.n > 0 and a.cnt > 0 and a.acc >= qq.q * a.n
		),
		-- Preserve request key order with ordered lists. Empty histograms emit each
		-- requested key with NULL value.
		dp_quantiles as (
			select t.id, to_json(map(
				list(t.q::varchar order by t.qi),
				list(double_wire_json(p.v) order by t.qi))) as quantiles
			from (select b.id, qq.q, qq.qi from dp_qsrc b,
				(select u.q, u.i as qi from unnest((select quantiles from input)) with ordinality u(q, i)) qq) t
			left join dp_q_picked p on p.id = t.id and p.q = t.q and p.rn = 1
			group by t.id
		),
		ts_dps_json as (
			select d.series_id,
				to_json(list(metric_datapoint_view_json(
					d,
					coalesce((select exemplars from exemplars_agg where exemplars_agg.metric_datapoint_id = d.id), json('[]')),
					coalesce((select exemplar_count from exemplars_agg where exemplars_agg.metric_datapoint_id = d.id), 0),
					dq.quantiles
				) order by d.timestamp desc, d.id)) as datapoints
			from shipped_dps d
			left join dp_quantiles dq on dq.id = d.id
			group by d.series_id
		),
		-- Pack series newest first with their exact parent Metric Resource payload.
		timeseries_agg as (
			select to_json(list(metric_series_view_json(
				t.attrs_key,
				t.attributes_sample,
				resource_json(s.resource_attribute_ids, s.resource_dropped_attributes_count),
				-- [] means this series shipped no datapoints.
				coalesce(tj.datapoints, json('[]')),
				series_stats_json(t.value_count, t.value_min, t.value_max, t.value_sum),
				swc.datapoint_count,
				swc.latest_ns::varchar,
				srs.rate_stats,
				sv.views,
				sp.sparkline
			-- Series ID makes equal latest timestamps deterministic.
			) order by t.latest_ts desc, t.attrs_key)) as timeseries
			from ts_dps_agg t
			-- Narrowed series have no datapoint JSON row.
			left join ts_dps_json tj on tj.series_id = t.series_id
			join metric_series ms on ms.id = t.series_id
			cross join selected_metric s
			-- Histograms and empty scalar series have no scalar views.
			left join scalar_views_agg sv on sv.series_id = t.series_id
			-- Sparklines are returned for every scalar series, including unchecked rows.
			left join sparkline_agg sp on sp.series_id = t.series_id
			-- Histograms have no rate stats.
			left join scalar_rate_stats srs on srs.series_id = t.series_id
			-- Every aggregated series has a window-count row.
			left join series_window_counts swc on swc.series_id = t.series_id
		),
		-- Merge reduced histogram series by bucket for heatmaps and summaries.
		-- Exclude any bucket already refused by a per-series bounds mismatch.
		agg_refused_buckets as (
			select distinct bucket_start
			from hist_folded
			where distinct_bounds > 1
		),
		agg_scaled as (
			select f.*, min(f.scale) over (partition by f.bucket_start) as agg_scale
			from hist_folded f
			where f.bucket_start not in (select bucket_start from agg_refused_buckets)
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
				-- Cross-series rows use the shared bucket start as timestamp.
				p.bucket_start as timestamp,
				min(p.start_time) as start_time,
				sum(p.count) as count,
				case when count(p.sum) = count(*) then sum(p.sum) end as sum,
				any_value(p.agg_scale) as scale,
				max(p.zero_threshold) as zero_threshold,
				sum(p.zero_count) as zero_count,
				any_value(coalesce(p.pos_agg_offset, 0)) as positive_bucket_offset,
				sum_bucket_vectors(list(p.pos_p)) as positive_bucket_counts,
				any_value(coalesce(p.neg_agg_offset, 0)) as negative_bucket_offset,
				sum_bucket_vectors(list(p.neg_p)) as negative_bucket_counts,
				-- hist_folded has normalized both temporalities to bucket activity.
				sum_bucket_vectors(list(p.bucket_counts)) as bucket_counts,
				any_value(p.explicit_bounds) as explicit_bounds,
				-- Explicit bounds must match across series.
				count(distinct p.explicit_bounds::varchar) as distinct_bounds
			from agg_padded p
			group by p.bucket_start
		),
		-- Fold buckets below the merged zero threshold into zero_count.
		agg_folds as (
			select m.*,
				zero_fold(m.positive_bucket_counts, m.positive_bucket_offset,
					m.zero_threshold, m.scale) as pos_fold,
				zero_fold(m.negative_bucket_counts, m.negative_bucket_offset,
					m.zero_threshold, m.scale) as neg_fold
			from agg_merged m
		),
		-- Report refused per-series and cross-series explicit-bounds merges.
		bounds_mismatch as (
			select
				(select count(*) from hist_folded where distinct_bounds > 1)
					as series_buckets,
				-- Include both per-series and cross-series incompatibilities.
				(select count(*) from agg_refused_buckets)
				+ (select count(*) from agg_folds where distinct_bounds > 1)
					as aggregate_buckets
		),
		-- Aggregate quantiles use the same relational bucket walk as datapoints.
		agg_qsrc as (
			select f.bucket_start,
				-- Non-NULL bounds identify explicit histograms; [] is one catch-all bucket.
				case when f.explicit_bounds is not null
					then hist_buckets(f.explicit_bounds, f.bucket_counts)
					else exp_buckets(f.scale,
						f.neg_fold."offset", f.neg_fold.counts,
						f.zero_count + f.pos_fold.folded + f.neg_fold.folded,
						f.pos_fold."offset", f.pos_fold.counts) end as buckets,
				(f.explicit_bounds is not null) as is_linear
			from agg_folds f
			where f.distinct_bounds <= 1
			  and len((select quantiles from input)) > 0
		),
		agg_q_acc as (
			select b.bucket_start, b.is_linear, u.b.lo as lo, u.b.hi as hi, u.b.cnt as cnt, u.i as i,
				coalesce(sum(u.b.cnt) over (partition by b.bucket_start order by u.i
					rows between unbounded preceding and 1 preceding), 0) as acc_prev,
				sum(u.b.cnt) over (partition by b.bucket_start order by u.i
					rows between unbounded preceding and current row) as acc,
				sum(u.b.cnt) over (partition by b.bucket_start) as n
			from agg_qsrc b, unnest(b.buckets) with ordinality u(b, i)
		),
		agg_q_picked as (
			select a.bucket_start, qq.q,
				case when a.is_linear
					then interp_linear(a.lo, a.hi, a.acc_prev, a.cnt, qq.q * a.n)
					else interp_loglin(a.lo, a.hi, a.acc_prev, a.cnt, qq.q * a.n) end as v,
				row_number() over (partition by a.bucket_start, qq.q order by a.i) as rn
			from agg_q_acc a, (select unnest((select quantiles from input)) as q) qq
			where a.n > 0 and a.cnt > 0 and a.acc >= qq.q * a.n
		),
		agg_quantiles as (
			select t.bucket_start, to_json(map(
				list(t.q::varchar order by t.qi),
				list(double_wire_json(p.v) order by t.qi))) as quantiles
			from (select b.bucket_start, qq.q, qq.qi from agg_qsrc b,
				(select u.q, u.i as qi from unnest((select quantiles from input)) with ordinality u(q, i)) qq) t
			left join agg_q_picked p on p.bucket_start = t.bucket_start and p.q = t.q and p.rn = 1
			group by t.bucket_start
		),
		aggregate_agg as (
			select to_json(list(metric_aggregate_bucket_view_json(
				f.timestamp, f.start_time, f.count, f.sum, f.scale,
				f.zero_threshold, f.zero_count, f.pos_fold, f.neg_fold,
				f.explicit_bounds, f.bucket_counts,
				aq.quantiles
			) order by f.bucket_start)) as aggregate
			from agg_folds f
			left join agg_quantiles aq on aq.bucket_start = f.bucket_start
			-- Do not merge incompatible explicit bounds.
			where f.distinct_bounds <= 1
		)
		-- Known Metrics return a row even when the window has no datapoints.
		select cast(json_object(
{{- if not .AggregateOnly}}
			'metricRef', s.id, 'name', s.name, 'description', s.description, 'unit', s.unit,
			'metadata', attrs_json(s.metadata_ids),
			'metricType', s.metric_type,
			'aggregationTemporalityCode', case
				when s.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then s.aggregation_temporality
				else null end,
			'aggregationTemporality', case
				when s.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then case s.aggregation_temporality
				when 0 then 'Unspecified' when 1 then 'Delta' when 2 then 'Cumulative'
				else 'Unknown (' || s.aggregation_temporality::varchar || ')' end else null end,
			'isMonotonic', case when s.metric_type = 'Sum' then s.is_monotonic else null end,
			'resourceDroppedAttributesCount', s.resource_dropped_attributes_count,
			'resourceSchemaUrl', s.resource_schema_url,
			'resource', resource_json(s.resource_attribute_ids,
				s.resource_dropped_attributes_count),
			'scopeName', s.scope_name, 'scopeVersion', s.scope_version,
			'scopeSchemaUrl', s.scope_schema_url,
			'scopeDroppedAttributesCount', s.scope_dropped_attributes_count,
			'scope', coalesce(
				scope_json(s.scope_name, s.scope_version, s.scope_attribute_ids,
					s.scope_dropped_attributes_count),
				json_object('name', s.scope_name, 'version', s.scope_version,
				            'attributes', json('[]'), 'droppedAttributesCount', 0)
			),
			'timeseries', coalesce((select timeseries from timeseries_agg), json('[]')),
{{- end}}
			-- NULL means no histogram merge; [] means a merge with no output buckets.
			'aggregate', {{if .NoHistogramMerge}}null{{else}}(select aggregate from aggregate_agg){{end}},
			-- Scalar pools use the per-series bucket shape. Histograms emit empty pools.
			'scalarAggregate', {{if .NoScalarPools}}json_object(
				'selected', json('[]'),
				'all', json('[]')
			){{else}}json_object(
				'selected', coalesce((select selected from scalar_pools_json), json('[]')),
				'all', coalesce((select all_series from scalar_pools_json), json('[]'))
			){{end}}{{if not .AggregateOnly}},
			-- Full-window latest timestamp and datapoint count before reduction.
			'lastSeenNs', (select max_ts from data_extent)::varchar,
			'datapointCount', coalesce((select sum(dp_count) from (
				select count(*) as dp_count from filtered_dps group by series_id
			)), 0),
			-- NULL boundsMismatch means no histogram merge was refused.
			'boundsMismatch', (
				select case when series_buckets > 0 or aggregate_buckets > 0
					then json_object(
						'seriesBuckets', series_buckets,
						'aggregateBuckets', aggregate_buckets)
				end
				from bounds_mismatch
			),
			'window', json_object(
				'requested', json_object(
					'startNs', (select time_start from input)::varchar,
					'endNs', (select time_end from input)::varchar
				),
				'effective', json_object(
					'startNs', (select start_ns from effective_window)::varchar,
					'endNs', (select end_ns from effective_window)::varchar
				)
			)
{{- end}}
		) as varchar) as metric
		from selected_metric s
