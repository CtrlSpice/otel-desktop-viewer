-- metrics stores one exact OTel Metric. Resource and Scope payloads are
-- referenced through their shared tables; description and metadata are received
-- Metric fields but do not participate in identity.
-- service_name is a derived search/display projection of Resource attributes.
-- metric_type preserves which descriptor fields are present; zero and false
-- fill fields that do not apply to that Metric type.
create table if not exists metrics (
		id uuid primary key,
		resource_id uuid not null,
		resource_payload_id uuid not null,
		scope_id uuid not null,
		name varchar not null,
		description varchar not null default '',
		unit varchar not null default '',
		metadata_ids uuid[] not null default [],
		metric_type varchar not null,
		-- Received OTLP enum number for Sum/Histogram/ExponentialHistogram.
		-- Gauge uses zero; metric_type makes that non-applicable value explicit.
		aggregation_temporality integer not null default 0,
		is_monotonic boolean not null default false,
		service_name varchar not null default '',
		foreign key (resource_id, resource_payload_id) references resources(id, payload_id),
		foreign key (scope_id) references scopes(id)
	)
