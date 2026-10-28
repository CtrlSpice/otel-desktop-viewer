create sequence if not exists metric_record_order;
-- Each received Metric owns its descriptor, context and nested points. The
-- metrics view exposes the latest descriptor for each logical identity.
-- Resource and Scope payloads are
-- owned by the Metric; description and metadata are received
-- Metric fields but do not participate in identity.
-- service_name is a derived search/display projection of Resource attributes.
-- metric_type preserves which descriptor fields are present; zero and false
-- fill fields that do not apply to that Metric type.
create table if not exists metric_records (
		id uuid not null,
		resource_id uuid not null,
		resource_payload_id uuid not null,
		scope_id uuid not null,
		name varchar not null,
		description varchar not null default '',
		unit varchar not null default '',
		metadata variant not null default '{}'::json::variant,
		metric_type varchar not null,
		-- Received OTLP enum number for Sum/Histogram/ExponentialHistogram.
		-- Gauge uses zero; metric_type makes that non-applicable value explicit.
		aggregation_temporality integer not null default 0,
		is_monotonic boolean not null default false,
		service_name varchar not null default '',
		resource otel_resource not null,
		scope otel_scope not null,
		record_id uuid primary key,
		received_order bigint default nextval('metric_record_order'),
		datapoints struct(
			id uuid, timestamp ubigint, start_time ubigint, flags uinteger,
			double_value double, int_value bigint, value_type varchar, count ubigint,
			sum double, min double, max double, bucket_counts ubigint[], explicit_bounds double[],
			scale integer, zero_count ubigint, zero_threshold double,
			positive_bucket_offset integer, positive_bucket_counts ubigint[],
			negative_bucket_offset integer, negative_bucket_counts ubigint[],
			attributes variant, attribute_key varchar,
			exemplars struct(id uuid, timestamp ubigint, double_value double, int_value bigint,
				trace_id uuid, span_id ubigint, attributes variant)[]
		)[],
		point_time_min ubigint,
		point_time_max ubigint
	);

create or replace view metrics as
select * exclude(record_id, received_order, datapoints, point_time_min, point_time_max)
from metric_records
qualify row_number() over(partition by id order by received_order desc)=1;
