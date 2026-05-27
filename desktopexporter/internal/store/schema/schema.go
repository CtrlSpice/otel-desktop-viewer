package schema

// Type creation queries
var TypeCreationQueries = []string{
	`create type attr_type as enum('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`,
}

// Table creation queries
// Order matters: spans before events/links, metric_streams before metric_ingests
// before datapoints before exemplars (FK dependencies). attributes references
// every owner table, so it goes last.
var TableCreationQueries = []string{
	`create table if not exists spans (
		trace_id uuid,
		trace_state varchar,
		span_id uuid primary key,
		parent_span_id uuid,
		name varchar,
		kind varchar,
		start_time bigint,
		end_time bigint,
		resource_dropped_attributes_count uinteger,
		scope_name varchar,
		scope_version varchar,
		scope_dropped_attributes_count uinteger,
		dropped_attributes_count uinteger,
		dropped_events_count uinteger,
		dropped_links_count uinteger,
		status_code varchar,
		status_message varchar,
		-- Denormalized cache of the resource attribute service.name (the
		-- single most-filtered-on column for span search). The source of
		-- truth is still the attributes row with key='service.name',
		-- scope='resource'; this column lets DuckDB's columnar storage and
		-- min-max indexes do equality filtering without a join.
		--
		-- NOT NULL with empty-string default: same rationale as
		-- metric_streams.service_name. The duckdb appender is also
		-- happier with a plain string column than with nullable typed
		-- pointers, which it doesn't accept directly.
		service_name varchar not null default ''
	)`,
	`create table if not exists events (
		id uuid primary key,
		span_id uuid not null,
		name varchar,
		timestamp bigint,
		dropped_attributes_count uinteger,
		foreign key (span_id) references spans(span_id)
	)`,
	`create table if not exists links (
		id uuid primary key,
		span_id uuid not null,
		trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		dropped_attributes_count uinteger,
		foreign key (span_id) references spans(span_id)
	)`,
	`create table if not exists logs (
		id uuid primary key,
		timestamp bigint,
		observed_timestamp bigint,
		trace_id uuid,
		span_id uuid,
		severity_text varchar,
		severity_number integer,
		body varchar,
		body_type varchar,
		resource_dropped_attributes_count uinteger,
		scope_name varchar,
		scope_version varchar,
		scope_dropped_attributes_count uinteger,
		dropped_attributes_count uinteger,
		flags uinteger,
		event_name varchar,
		-- See the matching service_name column on spans for rationale.
		service_name varchar not null default ''
	)`,
	// metric_streams is the canonical identity for a logical OTel metric.
	// Modeled after VictoriaMetrics's IndexDB pattern: every identity-bearing
	// query (Search, GetMetric, DeleteMetricStream, quantile/bucket series)
	// joins this table by surrogate UUID instead of reconstructing identity
	// from per-batch metric rows.
	//
	// The UNIQUE constraint on the 8-field tuple is what makes ingest's
	// find-or-insert correct: two OTLP batches that describe the same
	// logical stream produce the same id, so all their datapoints live
	// under one identity.
	//
	// service_name is part of identity (two metrics that share name+unit+...
	// but come from different services are different streams) and also acts
	// as the denormalized "filter by service" column for SearchSummaries.
	//
	// All eight identity columns are NOT NULL with empty-string ("" for
	// varchars, false for is_monotonic) defaults representing "not
	// applicable" (Gauge has no temporality/monotonicity, Histogram has
	// no monotonicity, etc.). This is a deliberate workaround for
	// DuckDB's standard-SQL behavior that treats two NULL values as
	// distinct in a UNIQUE constraint, which would defeat the
	// find-or-insert dedupe at ingest. The semantic distinction between
	// "unknown" and "not applicable" is borne by metric_type alone --
	// readers know that a Gauge's is_monotonic is N/A regardless of the
	// stored value.
	`create table if not exists metric_streams (
		id uuid primary key,
		name varchar not null,
		unit varchar not null default '',
		metric_type varchar not null,
		aggregation_temporality varchar not null default '',
		is_monotonic boolean not null default false,
		scope_name varchar not null default '',
		scope_version varchar not null default '',
		service_name varchar not null default '',
		unique (name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name)
	)`,
	// metric_ingests records each OTLP batch arrival for a stream. One row
	// per (stream, batch) -- so a long-lived counter that's reported every
	// 10s for an hour produces 360 metric_ingests rows pointing at one
	// metric_streams row. description and dropped-attribute counts can vary
	// across batches and are NOT identity, so they live here.
	`create table if not exists metric_ingests (
		id uuid primary key,
		stream_id uuid not null,
		description varchar,
		resource_dropped_attributes_count uinteger,
		scope_dropped_attributes_count uinteger,
		foreign key (stream_id) references metric_streams(id)
	)`,
	`create table if not exists datapoints (
		id uuid primary key,
		stream_id uuid not null,
		metric_ingest_id uuid not null,
		timestamp bigint,
		start_time bigint,
		flags uinteger,
		double_value double,
		int_value bigint,
		value_type varchar,
		count ubigint,
		sum double,
		min double,
		max double,
		bucket_counts ubigint[],
		explicit_bounds double[],
		scale integer,
		zero_count ubigint,
		zero_threshold double,
		positive_bucket_offset integer,
		positive_bucket_counts ubigint[],
		negative_bucket_offset integer,
		negative_bucket_counts ubigint[],
		-- Canonical "key=value|key=value|..." form of the datapoint's
		-- attribute set, with keys sorted ascending. Used for grouping
		-- datapoints by stream-within-stream (same metric, different
		-- attribute combinations) without a per-query string_agg over
		-- the attributes table. Computed in Go during the existing
		-- IngestAttributes pass so we don't iterate the attribute map
		-- twice. NULL for datapoints with no attributes.
		--
		-- Stored as a string (rather than a sha1 digest) so the column
		-- is self-describing when querying the DB directly, and so the
		-- frontend's chart-grouping code can compute the same string
		-- from raw attributes without hashing. Trade-off: variable-
		-- width column instead of fixed 20-byte. On a local tool with
		-- bounded retention this is the right side of the bargain.
		attrs_canonical varchar,
		foreign key (stream_id) references metric_streams(id),
		foreign key (metric_ingest_id) references metric_ingests(id)
	)`,
	`create table if not exists exemplars (
		id uuid primary key,
		datapoint_id uuid not null,
		timestamp bigint,
		value double,
		trace_id uuid,
		span_id uuid,
		foreign key (datapoint_id) references datapoints(id)
	)`,
	// attributes still references every owner table by FK; metric_id is
	// renamed to metric_ingest_id since the per-batch record (formerly
	// metrics) is now metric_ingests. The chk_attributes_one_owner
	// constraint is rewritten with the new column name; the exclusivity
	// rules are otherwise unchanged. Resource and scope attributes for a
	// metric live with the metric_ingest row (one set per ingest batch).
	`create table if not exists attributes (
		span_id uuid,
		event_id uuid,
		link_id uuid,
		log_id uuid,
		metric_ingest_id uuid,
		datapoint_id uuid,
		exemplar_id uuid,
		scope varchar not null,
		key varchar not null,
		value varchar not null,
		type attr_type not null,
		foreign key (span_id) references spans(span_id),
		foreign key (event_id) references events(id),
		foreign key (link_id) references links(id),
		foreign key (log_id) references logs(id),
		foreign key (metric_ingest_id) references metric_ingests(id),
		foreign key (datapoint_id) references datapoints(id),
		foreign key (exemplar_id) references exemplars(id),
		unique (span_id, event_id, link_id, log_id, metric_ingest_id, datapoint_id, exemplar_id, scope, key),
		constraint chk_attributes_one_owner check (
			(span_id is not null and event_id is null and link_id is null and log_id is null and metric_ingest_id is null and datapoint_id is null and exemplar_id is null) or
			(event_id is not null and span_id is not null and link_id is null and log_id is null and metric_ingest_id is null and datapoint_id is null and exemplar_id is null) or
			(link_id is not null and span_id is not null and event_id is null and log_id is null and metric_ingest_id is null and datapoint_id is null and exemplar_id is null) or
			(log_id is not null and span_id is null and event_id is null and link_id is null and metric_ingest_id is null and datapoint_id is null and exemplar_id is null) or
			(metric_ingest_id is not null and span_id is null and event_id is null and link_id is null and log_id is null and datapoint_id is null and exemplar_id is null) or
			(datapoint_id is not null and metric_ingest_id is not null and span_id is null and event_id is null and link_id is null and log_id is null and exemplar_id is null) or
			(exemplar_id is not null and datapoint_id is not null and metric_ingest_id is not null and span_id is null and event_id is null and link_id is null and log_id is null)
		)
	)`,
}

// Index creation queries.
//
// Three categories of changes vs. the pre-normalization schema:
//
//  1. Per-owner attribute indexes (idx_attributes_metric, the two
//     _hierarchy ones) are gone. They existed solely to speed up
//     identity-reconstruction CTEs that have themselves been deleted
//     because identity now lives in metric_streams.
//  2. Old metric indexes (idx_metrics_name/identity/received) are
//     replaced by the corresponding metric_streams / metric_ingests
//     indexes, sized for the new access patterns.
//  3. service_name indexes on spans/logs/metric_streams support direct
//     columnar filtering for "show me everything from service X" queries.
var IndexCreationQueries = []string{
	`create index if not exists idx_spans_traceid on spans(trace_id)`,
	`create index if not exists idx_spans_starttime on spans(start_time)`,
	`create index if not exists idx_spans_parentspanid on spans(parent_span_id)`,
	`create index if not exists idx_spans_service on spans(service_name)`,
	`create index if not exists idx_events_span on events(span_id)`,
	`create index if not exists idx_events_timestamp on events(timestamp)`,
	`create index if not exists idx_links_span on links(span_id)`,
	`create index if not exists idx_links_trace on links(trace_id, linked_span_id)`,
	`create index if not exists idx_logs_timestamp on logs(timestamp)`,
	`create index if not exists idx_logs_traceid on logs(trace_id)`,
	`create index if not exists idx_logs_severitynumber on logs(severity_number)`,
	`create index if not exists idx_logs_service on logs(service_name)`,
	`create index if not exists idx_metric_streams_name on metric_streams(name)`,
	`create index if not exists idx_metric_streams_service on metric_streams(service_name)`,
	`create index if not exists idx_metric_ingests_stream on metric_ingests(stream_id)`,
	`create index if not exists idx_datapoints_stream_time on datapoints(stream_id, timestamp desc)`,
	`create index if not exists idx_datapoints_stream_attrs on datapoints(stream_id, attrs_canonical)`,
	`create index if not exists idx_datapoints_time on datapoints(timestamp desc)`,
	`create index if not exists idx_exemplars_datapoint on exemplars(datapoint_id)`,
	`create index if not exists idx_exemplars_trace on exemplars(trace_id, span_id)`,
	`create index if not exists idx_attributes_span on attributes(span_id, key, value, type)`,
	`create index if not exists idx_attributes_event on attributes(event_id, key, value, type)`,
	`create index if not exists idx_attributes_link on attributes(link_id, key, value, type)`,
	`create index if not exists idx_attributes_log on attributes(log_id, key, value, type)`,
	`create index if not exists idx_attributes_datapoint on attributes(datapoint_id, key, value, type)`,
	`create index if not exists idx_attributes_exemplar on attributes(exemplar_id, key, value, type)`,
	`create index if not exists idx_attributes_metric_ingest on attributes(metric_ingest_id, key, value, type)`,
	`create index if not exists idx_attributes_key_value on attributes(key, value, type)`,
}

// Macro creation queries
// All macros use `create or replace` so re-init on existing databases is safe.
// Composition (top-level builds on bucket pipelines builds on builders + kernels):
//
//	interp_linear / interp_loglin           -- arithmetic kernels
//	hist_buckets / exp_*_buckets            -- shape-specific bucket builders
//	bucket_quantile_linear / _loglin        -- shared pipeline (cumulative -> filter -> kernel)
//	hist_quantile / exp_hist_quantile       -- top-level entry points
var MacroCreationQueries = []string{
	// Interpolation kernels.
	// interp_loglin falls back to linear when lo*hi <= 0 (zero endpoint or sign mismatch)
	`create or replace macro interp_linear(lo, hi, acc_prev, cnt, target) as (
		lo + (hi - lo) * (target - acc_prev) / cnt
	)`,

	`create or replace macro interp_loglin(lo, hi, acc_prev, cnt, target) as (
		case
			when lo = 0 or hi = 0 or sign(lo) <> sign(hi)
				then interp_linear(lo, hi, acc_prev, cnt, target)
			else lo * pow(hi / lo, (target - acc_prev) / cnt)
		end
	)`,

	// Bucket builders. Each emits a list of {lo, hi, cnt} structs in CDF walking order.
	// Cumulative counts are NOT computed here; bucket_quantile_* adds them.

	// Explicit-bound histogram. counts has len(bounds)+1 entries.
	// Open extreme buckets (i=1 and i=len(counts)) are clamped to bounds[1] / bounds[end]
	// so quantile interpolation in those regions returns the boundary value
	// (Prometheus convention; better than guessing an unbounded width).
	`create or replace macro hist_buckets(bounds, counts) as (
		list_transform(counts, lambda c, i: {
			'lo': case
					when i = 1 then bounds[1]
					when i = len(counts) then bounds[len(bounds)]
					else bounds[i - 1]
				  end,
			'hi': case
					when i = 1 then bounds[1]
					when i = len(counts) then bounds[len(bounds)]
					else bounds[i]
				  end,
			'cnt': c
		})
	)`,

	// Exponential histogram positive region. base = 2^(2^-scale).
	// Bucket at 1-based position i covers (base^(offset+i-1), base^(offset+i)].
	`create or replace macro exp_pos_buckets(scale, offset_, counts) as (
		list_transform(counts, lambda c, i: {
			'lo': pow(2.0, pow(2.0, -scale) * (offset_ + i - 1)),
			'hi': pow(2.0, pow(2.0, -scale) * (offset_ + i)),
			'cnt': c
		})
	)`,

	// Exponential histogram negative region, emitted in CDF order (most negative first).
	// Source bucket at original position j covers [-base^(offset+j), -base^(offset+j-1));
	// list_reverse walks j from len down to 1 so output is numerically ascending.
	//
	// Note: the OTLP wire format treats positives and negatives as independent
	// (not mirrored), but in practice the negative region is empty for the
	// common case (latency, byte counts, queue depth, ...). Only signed-value
	// instruments (temperature deltas, P&L, geo offsets) populate it. We handle
	// it correctly because the spec allows it and the formula is the same shape
	// as the positive region with sign-preserving math.
	`create or replace macro exp_neg_buckets(scale, offset_, counts) as (
		list_transform(list_reverse(counts), lambda c, i: {
			'lo': -pow(2.0, pow(2.0, -scale) * (offset_ + len(counts) - i + 1)),
			'hi': -pow(2.0, pow(2.0, -scale) * (offset_ + len(counts) - i)),
			'cnt': c
		})
	)`,

	// Zero bucket: always emit one entry to keep list_concat type-stable.
	// A zero-count entry is harmless: the filter step skips it (acc doesn't change).
	`create or replace macro exp_zero_bucket(zero_count) as (
		[{'lo': 0.0, 'hi': 0.0, 'cnt': coalesce(zero_count, 0)}]
	)`,

	// Three-region concat in CDF order: most-negative -> zero -> most-positive.
	// Nested 2-arg list_concat for portability.
	`create or replace macro exp_buckets(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts) as (
		list_concat(
			list_concat(
				exp_neg_buckets(scale, neg_offset, neg_counts),
				exp_zero_bucket(zero_count)
			),
			exp_pos_buckets(scale, pos_offset, pos_counts)
		)
	)`,

	// Shared quantile pipeline:
	//   1. params:    target = q * total
	//   2. with_acc:  attach acc_prev / acc to each bucket via list_transform with index
	//   3. chosen:    first bucket whose acc >= target
	//   4. interp:    apply linear or log-linear kernel
	//
	// O(N^2) cumulative is fine for OTel histograms (N <= 160 buckets).
	// The two macros are intentionally identical except for the kernel call (option A:
	// explicit duplication beats runtime indirection through a strategy tag).
	`create or replace macro bucket_quantile_linear(buckets, q) as (
		case
			when buckets is null or len(buckets) = 0 then null
			when coalesce(list_sum(list_transform(buckets, lambda b: b.cnt)), 0) <= 0 then null
			else (
				with
					params as (
						select q * list_sum(list_transform(buckets, lambda b: b.cnt)) as target
					),
					with_acc as (
						select list_transform(buckets, lambda b, i: {
							'lo': b.lo, 'hi': b.hi, 'cnt': b.cnt,
							'acc_prev': case when i = 1 then 0
								else list_sum(list_transform(list_slice(buckets, 1, i - 1), lambda x: x.cnt))
							end,
							'acc': list_sum(list_transform(list_slice(buckets, 1, i), lambda x: x.cnt))
						}) as bs
					),
					chosen as (
						select
							params.target as target,
							list_filter(with_acc.bs, lambda b: b.acc >= params.target)[1] as b
						from with_acc, params
					)
				select interp_linear(b.lo, b.hi, b.acc_prev, b.cnt, target) from chosen
			)
		end
	)`,

	`create or replace macro bucket_quantile_loglin(buckets, q) as (
		case
			when buckets is null or len(buckets) = 0 then null
			when coalesce(list_sum(list_transform(buckets, lambda b: b.cnt)), 0) <= 0 then null
			else (
				with
					params as (
						select q * list_sum(list_transform(buckets, lambda b: b.cnt)) as target
					),
					with_acc as (
						select list_transform(buckets, lambda b, i: {
							'lo': b.lo, 'hi': b.hi, 'cnt': b.cnt,
							'acc_prev': case when i = 1 then 0
								else list_sum(list_transform(list_slice(buckets, 1, i - 1), lambda x: x.cnt))
							end,
							'acc': list_sum(list_transform(list_slice(buckets, 1, i), lambda x: x.cnt))
						}) as bs
					),
					chosen as (
						select
							params.target as target,
							list_filter(with_acc.bs, lambda b: b.acc >= params.target)[1] as b
						from with_acc, params
					)
				select interp_loglin(b.lo, b.hi, b.acc_prev, b.cnt, target) from chosen
			)
		end
	)`,

	// Top-level convenience macros. All NULL/empty guards live here so callers
	// just see "give me a quantile, get null if it can't be computed".
	`create or replace macro hist_quantile(bounds, counts, q) as (
		case
			when bounds is null or counts is null or len(bounds) = 0 or len(counts) = 0 then null
			else bucket_quantile_linear(hist_buckets(bounds, counts), q)
		end
	)`,

	`create or replace macro exp_hist_quantile(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts, q) as (
		bucket_quantile_loglin(
			exp_buckets(scale, neg_offset, neg_counts, zero_count, pos_offset, pos_counts),
			q
		)
	)`,

	// floor_div: mathematical floor division that rounds toward negative
	// infinity. SQL's `/` (and DuckDB's integer divide) truncate toward zero,
	// which is wrong for downscaling exponential histograms with negative
	// bucket indices: e.g. floor(-3 / 2) = -2 (correct, bucket -3 belongs to
	// merged group -2), whereas trunc(-3 / 2) = -1 (wrong group).
	//
	// Cast through double to handle bigint inputs without integer-overflow
	// surprises at the boundaries; the floor result is then cast back to
	// bigint so callers can use it as an array index / offset.
	`create or replace macro floor_div(a, b) as (
		cast(floor(cast(a as double) / cast(b as double)) as bigint)
	)`,

	// downscale_exp_buckets: drop the resolution of an exponential histogram
	// by `levels` scale steps. A single "level" merges every pair of adjacent
	// buckets; level k merges 2^k adjacent buckets. Used during cross-stream
	// aggregation when streams arrive at different scales -- everyone gets
	// downscaled to the group's minimum scale before bucket-wise summation.
	//
	// Returns {offset: bigint, counts: bigint[]}. levels <= 0 (and null/empty
	// counts) is a no-op: input is returned unchanged. Negative levels would
	// require *upscaling*, which is not generally possible without losing
	// information about the original sub-bucket distribution.
	//
	// Approach: pair each input count with its 0-based position via list_zip,
	// then for each output bucket k in [new_offset, last_k] keep the inputs
	// whose original bucket index (offset_ + position) maps to k under
	// floor_div, and sum their counts. Single allocation per output bucket.
	//
	// Note on list_zip pair access: list_zip returns structs that DuckDB
	// treats as "unnamed" for .field access -- you have to index positionally
	// (pair[1], pair[2]) the same way sum_bucket_vectors does. The fields are
	// 1=count, 2=0-based position.
	// Implementation note: the macro body must NOT contain a subquery (no
	// `with`, no `select`). DuckDB refuses to bind subqueries that reference
	// macro parameters when the macro is called from a SELECT that itself
	// joins CTEs -- you get "Referenced table X not found! Candidate tables:
	// params". So the helper values factor / new_offset / last_k get
	// inlined; verbose but the planner is happy. Each subexpression is pure
	// arithmetic on the macro's parameters, so DuckDB folds the duplicates.
	`create or replace macro downscale_exp_buckets(counts, offset_, levels) as (
		case
			when counts is null or len(counts) = 0 or levels <= 0
				then {'offset': offset_, 'counts': counts}
			else {
				'offset': floor_div(offset_, cast(pow(2, levels) as bigint)),
				-- list_sum promotes to HUGEINT; cast back to BIGINT so the
				-- output type matches the input and downstream macros that
				-- expect bigint[] (sum_bucket_vectors, exp_pos_buckets, ...)
				-- don't trip on inferred-type mismatches.
				'counts': list_transform(
					range(
						0,
						floor_div(offset_ + len(counts) - 1, cast(pow(2, levels) as bigint))
							- floor_div(offset_, cast(pow(2, levels) as bigint))
							+ 1
					),
					k_off -> cast(
						coalesce(
							list_sum(
								list_transform(
									list_filter(
										list_zip(counts, range(0, len(counts))),
										pair -> floor_div(offset_ + pair[2], cast(pow(2, levels) as bigint))
											= floor_div(offset_, cast(pow(2, levels) as bigint)) + k_off
									),
									pair -> pair[1]
								)
							),
							0
						)
						as bigint
					)
				)
			}
		end
	)`,

	// fold_below_cutoff: after scale/offset alignment of an exponential
	// histogram aggregate, fold any leading buckets whose index is <= cutoff
	// into a single "folded" total. The folded value is intended to be added
	// back into zero_count by the caller, completing the zero_threshold
	// reconciliation step described in the histogram-trend-chart plan.
	//
	// Returns {counts: bigint[], offset: bigint, folded: bigint}. Where the
	// inputs trigger a no-op, folded is 0 and counts/offset pass through:
	//   - counts is NULL or empty
	//   - cutoff is NULL (signals "no zero_threshold to apply")
	//   - cutoff < offset_ (no buckets sit at or below the threshold)
	//
	// drop_n is capped by len(counts) so a wildly-high cutoff folds the whole
	// array rather than producing nonsense slices. list_slice in DuckDB is
	// 1-indexed and end-inclusive; both list_slice calls clamp gracefully on
	// out-of-range indices, so the cap is defensive rather than load-bearing.
	`create or replace macro fold_below_cutoff(counts, offset_, cutoff) as (
		case
			when counts is null or len(counts) = 0 or cutoff is null or cutoff < offset_
				then {'counts': counts, 'offset': offset_, 'folded': 0::bigint}
			else (
				with d as (
					select least(cutoff - offset_ + 1, len(counts)) as drop_n
				)
				select {
					'counts': list_slice(counts, drop_n + 1, len(counts)),
					'offset': offset_ + drop_n,
					'folded': cast(coalesce(list_sum(list_slice(counts, 1, drop_n)), 0) as bigint)
				}
				from d
			)
		end
	)`,

	// pad_left_to_offset: left-pads `counts` with zeros so the first bucket
	// lines up with `target_offset`. Used during cross-stream exp-histogram
	// alignment after downscaling: every stream is downscaled to the group's
	// minimum scale, then padded so every aligned bucket array starts at the
	// same (minimum) offset.
	//
	// Caller invariant is target_offset <= current_offset (you can only ever
	// extend a bucket array's coverage downward, never trim it). When the
	// invariant is violated or padding is unnecessary (target == current),
	// returns counts unchanged. NULL counts pass through.
	//
	// Implementation note: DuckDB doesn't have list_repeat(value, n) in this
	// version, so the zero prefix is built via list_transform(range(0, n)).
	// The 0::bigint cast keeps the prefix type aligned with bigint[] inputs
	// so list_concat doesn't fail on a bigint-vs-int mismatch.
	`create or replace macro pad_left_to_offset(counts, current_offset, target_offset) as (
		case
			when counts is null or current_offset <= target_offset then counts
			else list_concat(
				list_transform(range(0, current_offset - target_offset), x -> 0::bigint),
				counts
			)
		end
	)`,

	// Aggregation helper: element-wise sum of a list of equal-length numeric
	// lists. Used to merge bucket_counts arrays across multiple histogram
	// streams that share the same explicit_bounds. The caller is responsible
	// for enforcing the shared-bounds invariant; this macro is intentionally
	// permissive about length mismatches (zero-pads via list_zip + coalesce)
	// so a programmer error there yields slightly-off numbers rather than a
	// crash.
	//
	// Returns NULL for NULL or empty input -- DuckDB's list_reduce raises a
	// hard error on an empty list, so we guard explicitly. NULL slots inside
	// an element list are coalesced to 0.
	`create or replace macro sum_bucket_vectors(vectors) as (
		case
			when vectors is null or len(vectors) = 0 then null
			else list_reduce(
				vectors,
				(acc, v) -> list_transform(
					list_zip(acc, v),
					pair -> coalesce(pair[1], 0) + coalesce(pair[2], 0)
				)
			)
		end
	)`,
}
