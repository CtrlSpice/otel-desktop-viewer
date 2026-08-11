package schema

// Type creation queries
var TypeCreationQueries = []string{
	`create type attr_type as enum('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`,
}

// Table creation queries
//
// Order matters (FK dependencies): attributes first -- it references nothing --
// then resources/scopes, then the signals that reference them, then
// metric_streams before metric_ingests before datapoints before exemplars.
//
// This inverts the previous order, where attributes went last because it
// carried an FK to every owner table. It now owns no ownership at all: owners
// point at it, by uuid[], and DuckDB cannot FK into a LIST so those references
// are unenforced by construction. See dictionary integrity in the store tests.
var TableCreationQueries = []string{
	// The attribute dictionary: one row per distinct (key, value, type, scope)
	// for the whole database. On the reference capture, 723,692 attribute rows
	// collapse to 267 dictionary rows.
	//
	// id = sha256 over the length-prefixed fields, truncated to 16 bytes.
	// Content-derived rather than surrogate, so ingest can compute it without
	// asking the database and the owners' arrays are correct by construction.
	// Identity being the primary key is also why there is no UNIQUE here: it
	// would be redundant against the PK, and would put an index over `value`,
	// which holds whole SQL statements and stack traces.
	//
	// `scope` is part of identity, not a free-form tag. Attribute discovery has
	// to report attributeScope (a closed union in wire-types.ts), and scope
	// otherwise lives only in *which* array references an id -- unrecoverable
	// without unnesting every owner. The cost is that the same triple under two
	// scopes is two rows, which is negligible: the populations barely overlap.
	`create table if not exists attributes (
		id uuid primary key,
		key varchar not null,
		value varchar not null,
		type attr_type not null,
		scope varchar not null
	)`,

	// seq is the short, store-stable key the wire format uses instead of a
	// 36-char uuid. Sequence values are never reused, so a client cache can
	// miss but never be wrong.
	`create sequence if not exists resource_seq`,
	`create sequence if not exists scope_seq`,

	// id = sha256(attribute_ids, dropped_attributes_count).
	//
	// dropped_attributes_count is part of identity, so a resource with the same
	// attributes but a different dropped count is a separate row. Correct, but
	// note an exporter that varies its dropped count fragments the dedupe.
	`create table if not exists resources (
		id uuid primary key,
		seq integer not null default nextval('resource_seq'),
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)`,

	// id = sha256(name, version, attribute_ids, dropped_attributes_count).
	`create table if not exists scopes (
		id uuid primary key,
		seq integer not null default nextval('scope_seq'),
		name varchar not null default '',
		version varchar not null default '',
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)`,

	`create table if not exists spans (
		trace_id uuid,
		trace_state varchar,
		span_id uuid primary key,
		parent_span_id uuid,
		name varchar,
		kind varchar,
		start_time bigint,
		end_time bigint,
		resource_id uuid not null,
		scope_id uuid not null,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		dropped_events_count uinteger,
		dropped_links_count uinteger,
		status_code varchar,
		status_message varchar,
		-- Denormalized cache of the resource attribute service.name (the
		-- single most-filtered-on column for span search). The source of
		-- truth is still the attribute row reached through resource_id;
		-- this column lets DuckDB's columnar storage and min-max indexes
		-- do equality filtering without a join.
		--
		-- Kept despite resources now being deduped: with ~24 resource rows
		-- the join is cheap, but this is the hottest filter in span search
		-- and a column scan still beats a join plus an array unnest.
		--
		-- NOT NULL with empty-string default: same rationale as
		-- metric_streams.service_name. The duckdb appender is also
		-- happier with a plain string column than with nullable typed
		-- pointers, which it doesn't accept directly.
		service_name varchar not null default '',
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)`,
	`create table if not exists events (
		id uuid primary key,
		span_id uuid not null,
		name varchar,
		timestamp bigint,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		foreign key (span_id) references spans(span_id)
	)`,
	`create table if not exists links (
		id uuid primary key,
		span_id uuid not null,
		trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		attribute_ids uuid[] not null,
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
		resource_id uuid not null,
		scope_id uuid not null,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		flags uinteger,
		event_name varchar,
		-- See the matching service_name column on spans for rationale.
		service_name varchar not null default '',
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
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
	// metric_series is the timeseries: the thing a chart draws one line per.
	//
	// The schema had no row for it. metric_streams is deliberately coarser --
	// it identifies a stream by service_name rather than by resource, so a
	// counter stays one timeseries when a pod restarts -- and the actual series
	// was only ever implied, by grouping datapoints on their label set at query
	// time. That had three costs:
	//
	//   - Two replicas of one service, emitting the same instrument with the
	//     same labels, grouped together and interleaved into one line.
	//   - Grouping ran on a LIST column, which DuckDB cannot index and must
	//     rebuild and hash per row. Measured on 294,607 datapoints: 5.0ms
	//     grouping by the array against 0.9ms by a fixed-width key.
	//   - A series had no name, so nothing could link to one. Metric URLs could
	//     only reference a datapoint id, which retention deletes -- shared links
	//     rotted, and degraded silently to "no selection".
	//
	// id = sha256(stream_id, resource_id, attribute_ids), so it is stable
	// across restarts and re-ingests: the same series always has the same id,
	// which is what makes it safe to put in a URL.
	`create table if not exists metric_series (
		id uuid primary key,
		stream_id uuid not null,
		resource_id uuid not null,
		attribute_ids uuid[] not null,
		foreign key (stream_id) references metric_streams(id),
		foreign key (resource_id) references resources(id)
	)`,

	// metric_ingests records each OTLP batch arrival for a stream. One row
	// per (stream, batch) -- so a long-lived counter that's reported every
	// 10s for an hour produces 360 metric_ingests rows pointing at one
	// metric_streams row. description varies across batches and is NOT
	// identity, so it lives here; resource and scope are now references
	// rather than per-batch dropped counts.
	`create table if not exists metric_ingests (
		id uuid primary key,
		stream_id uuid not null,
		description varchar,
		resource_id uuid not null,
		scope_id uuid not null,
		foreign key (stream_id) references metric_streams(id),
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)`,
	`create table if not exists datapoints (
		id uuid primary key,
		stream_id uuid not null,
		-- The series this point belongs to. Grouping a chart is now an
		-- equality on one fixed-width column instead of hashing a LIST, and
		-- it is indexable, which a LIST is not.
		--
		-- attribute_ids stays alongside it rather than being replaced: it is
		-- what the series id is derived from, it is what attrs_json renders,
		-- and dropping it would make the labels of a datapoint unreachable
		-- without a join back through metric_series.
		series_id uuid not null,
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
		-- Replaces attrs_canonical. That column materialised the datapoint's
		-- attribute set as "key=value|..." so grouping by stream-within-stream
		-- was an equality compare on a varchar. The array serves the same
		-- purpose and is the identity itself rather than a rendering of it:
		-- equal arrays mean equal attribute sets, by construction.
		--
		-- This is where the rewrite pays most. On the reference capture,
		-- 294,607 datapoints carry 591,890 attribute rows resolving to 89
		-- distinct label sets -- 82% of the whole attributes table.
		attribute_ids uuid[] not null,
		foreign key (stream_id) references metric_streams(id),
		foreign key (series_id) references metric_series(id),
		foreign key (metric_ingest_id) references metric_ingests(id)
	)`,
	`create table if not exists exemplars (
		id uuid primary key,
		datapoint_id uuid not null,
		timestamp bigint,
		value double,
		trace_id uuid,
		span_id uuid,
		attribute_ids uuid[] not null,
		foreign key (datapoint_id) references datapoints(id)
	)`,
}

// Index creation queries.
//
// Changes vs. the pre-dictionary schema:
//
//  1. The eight per-owner attribute indexes are gone, and nothing replaces
//     them. `attributes` is now a ~267-row dictionary with a primary key;
//     there is nothing left to index. idx_attributes_key_value goes too:
//     global text search now scans a few hundred rows instead of ~700k.
//  2. idx_datapoints_stream_attrs is gone. It indexed (stream_id,
//     attrs_canonical), and DuckDB cannot index a LIST column, so the
//     attribute_ids replacement has no equivalent. Grouping by the array
//     still works, unindexed, and is already narrowed by stream_id via
//     idx_datapoints_stream_time. Confirm on a metrics-heavy session.
//  3. New resource/scope reference indexes, for the joins that replace the
//     old per-span attribute lookups.
//
// Note what cannot be indexed at all: attribute_ids. Attribute search is a
// list_contains probe over the candidate set rather than an index seek, but
// the candidate set is already narrowed by time range, and the probe is an
// inline array test rather than a correlated subquery into a huge table.
// Indexes here are ART indexes, and ART indexes in DuckDB serve exactly one
// thing: equality and IN(...) on a single column. They do not accelerate range
// predicates, joins, aggregation or sorting -- DuckDB uses hash joins and
// zonemaps for those.
//
// So a time-window filter gets nothing from an index on the time column, which
// is why there are none here. Four such indexes were removed after measuring:
// idx_spans_starttime, idx_events_timestamp, idx_logs_timestamp and
// idx_datapoints_time. On the reference capture the range scan they were
// supposed to serve ran in 0.085ms with the index and 0.088ms without --
// alternating the arms so neither got a warmer cache -- while they cost insert
// throughput and memory on every write.
//
// The fallback argument does not rescue them either. Zonemaps prune by row-group
// min/max and only work when data is physically ordered by the filtered column;
// spans are exported when they *end*, so a long-running parent arrives last with
// the earliest start_time. Measured on the race replay: 25.8% of rows have a
// start_time lower than the preceding row's. Scrambled arrival means the zonemap
// cannot prune well and the ART index cannot serve the range -- so neither
// mechanism helps, and only the write cost is real.
//
// Do not re-add a time-column index without a measurement showing it helps.
var IndexCreationQueries = []string{
	`create index if not exists idx_spans_traceid on spans(trace_id)`,
	`create index if not exists idx_spans_parentspanid on spans(parent_span_id)`,
	`create index if not exists idx_spans_service on spans(service_name)`,
	`create index if not exists idx_spans_resource on spans(resource_id)`,
	`create index if not exists idx_spans_scope on spans(scope_id)`,
	`create index if not exists idx_events_span on events(span_id)`,
	`create index if not exists idx_links_span on links(span_id)`,
	`create index if not exists idx_links_trace on links(trace_id, linked_span_id)`,
	`create index if not exists idx_logs_traceid on logs(trace_id)`,
	`create index if not exists idx_logs_severitynumber on logs(severity_number)`,
	`create index if not exists idx_logs_service on logs(service_name)`,
	`create index if not exists idx_logs_resource on logs(resource_id)`,
	`create index if not exists idx_logs_scope on logs(scope_id)`,
	`create index if not exists idx_metric_streams_name on metric_streams(name)`,
	`create index if not exists idx_metric_streams_service on metric_streams(service_name)`,
	`create index if not exists idx_metric_ingests_stream on metric_ingests(stream_id)`,
	`create index if not exists idx_metric_ingests_resource on metric_ingests(resource_id)`,
	`create index if not exists idx_datapoints_stream_time on datapoints(stream_id, timestamp desc)`,
	// Restores what idx_datapoints_stream_attrs used to do before attributes
	// became a LIST. ART indexes serve equality on a single column, which is
	// exactly the shape series grouping now has.
	`create index if not exists idx_datapoints_series on datapoints(series_id)`,
	`create index if not exists idx_metric_series_stream on metric_series(stream_id)`,
	`create index if not exists idx_exemplars_datapoint on exemplars(datapoint_id)`,
	`create index if not exists idx_exemplars_trace on exemplars(trace_id, span_id)`,
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
	// attr_frame / attr_id mirror ingest.AttributeID in SQL.
	//
	// This is a deliberate second implementation, not shared code. A Go-side
	// re-hash would use the very function that wrote the ids and could only
	// ever catch storage corruption; an independent one also catches a bug in
	// the Go hashing, and catches the encoding drifting between builds -- the
	// failure mode that is far likelier than a 128-bit collision.
	//
	// Two traps, both found by testing rather than reading docs:
	//   - strlen() is byte length and matches Go's len(); length() counts
	//     characters and would diverge on any non-ASCII value.
	//   - k::blob is not a usable way to get byte length: DuckDB rejects
	//     non-ASCII in a VARCHAR->BLOB cast.
	//
	// Verified against an independent shasum on ASCII and UTF-8 input.
	`create or replace macro attr_frame(k, v, t, s) as (
		strlen(k)::varchar || ':' || k ||
		strlen(v)::varchar || ':' || v ||
		strlen(t)::varchar || ':' || t ||
		strlen(s)::varchar || ':' || s
	)`,
	`create or replace macro attr_id(k, v, t, s) as (
		cast(
			substr(sha256(attr_frame(k,v,t,s)),  1, 8) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)),  9, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 13, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 17, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 21, 12)
		as uuid)
	)`,

	// attrs_json renders an owner's attribute_ids as the wire attribute array.
	//
	// Replaces the per-owner attribute CTE that appeared eight times across
	// spans.go, logs.go and metrics.go. Ordered by key: array order is
	// identity, not presentation, so display order is imposed here.
	`create or replace macro attrs_json(ids) as (
		coalesce((
			select to_json(list(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)
			                    order by a.key, a.id))
			from unnest(ids) as t(aid)
			join attributes a on a.id = t.aid
		), json('[]'))
	)`,

	// attrs_key renders an attribute set as the canonical "key=value|..."
	// string, keys in lexicographic order.
	//
	// This is the old datapoints.attrs_canonical column, computed on demand
	// from attribute_ids instead of materialised at ingest. It survives only
	// because JsonMetricTimeseries.attributesKey is still that string on the
	// wire; when series identity gains the resource, this becomes a composite
	// and the macro goes with it.
	//
	// Not an identity primitive any more: grouping is by attribute_ids, which
	// is the identity itself. This only renders it.
	`create or replace macro attrs_key(ids) as (
		coalesce((
			select string_agg(a.key || '=' || a.value, '|' order by a.key, a.id)
			from unnest(ids) as t(aid)
			join attributes a on a.id = t.aid
		), '')
	)`,

	// attr_value looks one attribute up by key, NULL when absent.
	//
	// This is how resource.* and scope.* attribute *searches* resolve, now that
	// there is no per-owner attributes row to correlate against: the array is
	// already on the joined resources / scopes row, and that table is a couple
	// of dozen rows, so the lookup is effectively cached.
	//
	// Note it is NOT how service.name is read on the hot filter path.
	// spans.service_name and logs.service_name were kept as denormalized
	// columns (the plan had proposed dropping them); a column scan still beats
	// a join plus an unnest for the single most-filtered-on field. This macro
	// is what verifies those columns agree with the resource they came from.
	`create or replace macro attr_value(ids, k) as (
		(select a.value
		 from unnest(ids) as t(aid)
		 join attributes a on a.id = t.aid
		 where a.key = k
		 limit 1)
	)`,

	// has_attr tests membership by id. An inline array probe -- no join, no
	// correlated subquery into a table of owner-attribute rows.
	//
	// Callers pass an id computed in Go by ingest.AttributeID, which is why
	// exact-match attribute search needs no database round trip to build its
	// predicate. Deliberately not attr_id(...) inline: that would put the audit
	// macro on the correctness path, where a Go/SQL divergence turns into search
	// quietly returning nothing instead of a failing test.
	`create or replace macro has_attr(ids, id) as (
		list_contains(ids, id)
	)`,

	// Wire renderings of the two id types. Trace ids go out as 32 hex chars and
	// span ids as the low 16, matching the OTLP/JSON convention.
	`create or replace macro trace_id_wire(id) as (
		replace(id::varchar, '-', '')
	)`,
	`create or replace macro span_id_wire(id) as (
		right(replace(id::varchar, '-', ''), 16)
	)`,

	// Component objects. resource_json / scope_json take the row's own fields
	// rather than an id, so a caller that already joined the row needs no
	// second lookup.
	`create or replace macro resource_json(ids, dropped) as (
		json_object('attributes', attrs_json(ids), 'droppedAttributesCount', dropped)
	)`,
	`create or replace macro scope_json(name, version, ids, dropped) as (
		json_object('name', name, 'version', version,
		            'attributes', attrs_json(ids), 'droppedAttributesCount', dropped)
	)`,

	// One attribute *definition* -- the shape the search-field dropdowns read.
	// Identical in four places before this: two in spans.go, one each in
	// logs.go and metrics.go.
	`create or replace macro attribute_def_json(key, scope, type) as (
		json_object('name', key, 'attributeScope', scope, 'type', type::varchar)
	)`,

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
