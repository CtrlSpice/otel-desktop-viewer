-- metric_streams is the canonical identity for a logical OTel metric.
-- Modeled after VictoriaMetrics's IndexDB pattern: every identity-bearing
-- query (Search, GetMetric, DeleteMetricStream, quantile/bucket series)
-- joins this table by surrogate UUID instead of reconstructing identity
-- from per-batch metric rows.
--
-- The UNIQUE constraint on the 8-field tuple is what makes ingest's
-- find-or-insert correct: two OTLP batches that describe the same
-- logical stream produce the same id, so all their datapoints live
-- under one identity.
--
-- service_name is part of identity (two metrics that share name+unit+...
-- but come from different services are different streams) and also acts
-- as the denormalized "filter by service" column for SearchSummaries.
--
-- All eight identity columns are NOT NULL with empty-string ("" for
-- varchars, false for is_monotonic) defaults representing "not
-- applicable" (Gauge has no temporality/monotonicity, Histogram has
-- no monotonicity, etc.). This is a deliberate workaround for
-- DuckDB's standard-SQL behavior that treats two NULL values as
-- distinct in a UNIQUE constraint, which would defeat the
-- find-or-insert dedupe at ingest. The semantic distinction between
-- "unknown" and "not applicable" is borne by metric_type alone --
-- readers know that a Gauge's is_monotonic is N/A regardless of the
-- stored value.
create table if not exists metric_streams (
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
	)
