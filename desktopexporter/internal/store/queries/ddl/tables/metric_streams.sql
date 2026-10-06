-- metric_streams is the canonical identity for one exact OTel Metric.
-- The primary key is a versioned content ID over complete Resource attributes,
-- the complete InstrumentationScope tuple, and the identifying Metric
-- descriptor fields. Ingest compares this stored tuple on every ID conflict.
-- Resource and Scope schema/dropped-count payload variants remain on
-- metric_ingests; only ScopeMetrics.schema_url participates in identity.
-- service_name is a derived search/display projection of Resource attributes.
-- Placeholder zero/false descriptor values are non-applicable according to
-- metric_type and do not represent received values for Gauge or Summary.
create table if not exists metric_streams (
		id uuid primary key,
		resource_attribute_ids uuid[] not null default [],
		name varchar not null,
		unit varchar not null default '',
		metric_type varchar not null,
		-- Received OTLP enum number for Sum/Histogram/ExponentialHistogram.
		-- Gauge uses zero; metric_type makes that non-applicable value explicit.
		aggregation_temporality integer not null default 0,
		is_monotonic boolean not null default false,
		scope_name varchar not null default '',
		scope_version varchar not null default '',
		scope_schema_url varchar not null default '',
		scope_attribute_ids uuid[] not null default [],
		service_name varchar not null default '',
	)
