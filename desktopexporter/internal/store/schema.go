package store

// Type creation queries
var TypeCreationQueries = []string{
	`create type attr_type as enum('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`,
}

// Table creation queries
// Order matters: spans before events/links, metrics before datapoints, datapoints before exemplars (FK dependencies)
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
		search_text varchar
	)`,
	`create table if not exists events (
		id uuid primary key,
		span_id uuid not null,
		name varchar,
		timestamp bigint,
		dropped_attributes_count uinteger,
		search_text varchar,
		foreign key (span_id) references spans(span_id)
	)`,
	`create table if not exists links (
		id uuid primary key,
		span_id uuid not null,
		trace_id uuid,
		linked_span_id uuid,
		trace_state varchar,
		dropped_attributes_count uinteger,
		search_text varchar,
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
		search_text varchar
	)`,
	`create table if not exists metrics (
		id uuid primary key,
		name varchar,
		description varchar,
		unit varchar,
		resource_dropped_attributes_count uinteger,
		scope_name varchar,
		scope_version varchar,
		scope_dropped_attributes_count uinteger,
		received bigint,
		search_text varchar
	)`,
	`create table if not exists datapoints (
		id uuid primary key,
		metric_id uuid not null,
		metric_type varchar not null,
		timestamp bigint,
		start_time bigint,
		flags uinteger,
		double_value double,
		int_value bigint,
		value_type varchar,
		is_monotonic boolean,
		aggregation_temporality varchar,
		count ubigint,
		sum double,
		min double,
		max double,
		bucket_counts ubigint[],
		explicit_bounds double[],
		scale integer,
		zero_count ubigint,
		positive_bucket_offset integer,
		positive_bucket_counts ubigint[],
		negative_bucket_offset integer,
		negative_bucket_counts ubigint[],
		foreign key (metric_id) references metrics(id)
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
	`create table if not exists attributes (
		span_id uuid,
		event_id uuid,
		link_id uuid,
		log_id uuid,
		metric_id uuid,
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
		foreign key (metric_id) references metrics(id),
		foreign key (datapoint_id) references datapoints(id),
		foreign key (exemplar_id) references exemplars(id),
		unique (span_id, event_id, link_id, log_id, metric_id, datapoint_id, exemplar_id, scope, key)
	)`,
}

// Constraint creation queries for discriminated union enforcement
var ConstraintCreationQueries = []string{
	`alter table attributes add constraint chk_attributes_one_owner check (
		(span_id is not null and event_id is null and link_id is null and log_id is null and metric_id is null and datapoint_id is null and exemplar_id is null) or
		(event_id is not null and span_id is not null and link_id is null and log_id is null and metric_id is null and datapoint_id is null and exemplar_id is null) or
		(link_id is not null and span_id is not null and event_id is null and log_id is null and metric_id is null and datapoint_id is null and exemplar_id is null) or
		(log_id is not null and span_id is null and event_id is null and link_id is null and metric_id is null and datapoint_id is null and exemplar_id is null) or
		(metric_id is not null and span_id is null and event_id is null and link_id is null and log_id is null and datapoint_id is null and exemplar_id is null) or
		(datapoint_id is not null and metric_id is not null and span_id is null and event_id is null and link_id is null and log_id is null and exemplar_id is null) or
		(exemplar_id is not null and datapoint_id is not null and metric_id is not null and span_id is null and event_id is null and link_id is null and log_id is null)
	)`,
	`alter table datapoints add constraint chk_metric_type_valid check (
		metric_type in ('Gauge', 'Sum', 'Histogram', 'ExponentialHistogram', 'Empty')
	)`,
	`alter table datapoints add constraint chk_empty_fields check (
		(metric_type != 'Empty') or (
			double_value is null and int_value is null and value_type is null AND
			is_monotonic is null and aggregation_temporality is null AND
			count is null and sum is null and min is null and max is null AND
			bucket_counts is null and explicit_bounds is null AND
			scale is null and zero_count is null AND
			positive_bucket_offset is null and positive_bucket_counts is null AND
			negative_bucket_offset is null and negative_bucket_counts is null
		)
	)`,
	`alter table datapoints add constraint chk_gauge_fields check (
		(metric_type != 'Gauge') or (
			value_type is not null and (double_value is not null or int_value is not null) AND
			count is null and sum is null and min is null and max is null AND
			bucket_counts is null and explicit_bounds is null AND
			scale is null and zero_count is null AND
			positive_bucket_offset is null and positive_bucket_counts is null AND
			negative_bucket_offset is null and negative_bucket_counts is null AND
			aggregation_temporality is null
		)
	)`,
	`alter table datapoints add constraint chk_sum_fields check (
		(metric_type != 'Sum') or (
			value_type is not null and (double_value is not null or int_value is not null) AND
			is_monotonic is not null and aggregation_temporality is not null AND
			count is null and sum is null and min is null and max is null AND
			bucket_counts is null and explicit_bounds is null AND
			scale is null and zero_count is null AND
			positive_bucket_offset is null and positive_bucket_counts is null AND
			negative_bucket_offset is null and negative_bucket_counts is null
		)
	)`,
	`alter table datapoints add constraint chk_histogram_fields check (
		(metric_type != 'Histogram') or (
			count is not null and sum is not null AND
			bucket_counts is not null and explicit_bounds is not null AND
			aggregation_temporality is not null AND
			double_value is null and int_value is null and value_type is null and is_monotonic is null AND
			scale is null and zero_count is null AND
			positive_bucket_offset is null and positive_bucket_counts is null AND
			negative_bucket_offset is null and negative_bucket_counts is null
		)
	)`,
	`alter table datapoints add constraint chk_exponential_histogram_fields check (
		(metric_type != 'ExponentialHistogram') or (
			count is not null and sum is not null AND
			scale is not null and zero_count is null AND
			positive_bucket_offset is not null and positive_bucket_counts is null AND
			negative_bucket_offset is not null and negative_bucket_counts is null AND
			aggregation_temporality is not null AND
			double_value is null and int_value is null and value_type is null and is_monotonic is null AND
			bucket_counts is null and explicit_bounds is null
		)
	)`,
}

// Index creation queries
var IndexCreationQueries = []string{
	`create index if not exists idx_spans_traceid on spans(trace_id)`,
	`create index if not exists idx_spans_starttime on spans(start_time)`,
	`create index if not exists idx_spans_parentspanid on spans(parent_span_id)`,
	`create index if not exists idx_events_span on events(span_id)`,
	`create index if not exists idx_events_timestamp on events(timestamp)`,
	`create index if not exists idx_links_span on links(span_id)`,
	`create index if not exists idx_links_trace on links(trace_id, linked_span_id)`,
	`create index if not exists idx_logs_timestamp on logs(timestamp)`,
	`create index if not exists idx_logs_traceid on logs(trace_id)`,
	`create index if not exists idx_logs_severitynumber on logs(severity_number)`,
	`create index if not exists idx_logs_searchtext on logs(search_text)`,
	`create index if not exists idx_metrics_name on metrics(name)`,
	`create index if not exists idx_metrics_received on metrics(received)`,
	`create index if not exists idx_metrics_searchtext on metrics(search_text)`,
	`create index if not exists idx_datapoints_type_metric_time on datapoints(metric_type, metric_id, timestamp desc)`,
	`create index if not exists idx_datapoints_metric_time on datapoints(metric_id, timestamp desc)`,
	`create index if not exists idx_datapoints_time on datapoints(timestamp desc)`,
	`create index if not exists idx_exemplars_datapoint on exemplars(datapoint_id)`,
	`create index if not exists idx_exemplars_trace on exemplars(trace_id, span_id)`,
	`create index if not exists idx_attributes_span on attributes(span_id, key, value, type)`,
	`create index if not exists idx_attributes_event on attributes(event_id, key, value, type)`,
	`create index if not exists idx_attributes_link on attributes(link_id, key, value, type)`,
	`create index if not exists idx_attributes_log on attributes(log_id, key, value, type)`,
	`create index if not exists idx_attributes_metric on attributes(metric_id, key, value, type)`,
	`create index if not exists idx_attributes_datapoint on attributes(datapoint_id, key, value, type)`,
	`create index if not exists idx_attributes_exemplar on attributes(exemplar_id, key, value, type)`,
	`create index if not exists idx_attributes_span_hierarchy on attributes(span_id, event_id, link_id)`,
	`create index if not exists idx_attributes_metric_hierarchy on attributes(metric_id, datapoint_id, exemplar_id)`,
	`create index if not exists idx_attributes_key_value on attributes(key, value, type)`,
}
