-- metric_ingests records each OTLP batch arrival for a stream. One row
-- per (stream, batch) -- so a long-lived counter that's reported every
-- 10s for an hour produces 360 metric_ingests rows pointing at one
-- metric_streams row. description varies across batches and is NOT
-- identity, so it lives here; resource and scope are now references
-- rather than per-batch dropped counts.
create table if not exists metric_ingests (
		id uuid primary key,
		stream_id uuid not null,
		description varchar,
		-- Metric.metadata: an attribute map on the metric itself, not on its
		-- datapoints. Lives here beside description for the same reason --
		-- it can differ between batches of one stream and is not identity.
		metadata_ids uuid[] not null default [],
		resource_id uuid not null,
		scope_id uuid not null,
		-- Resource schema URL is not part of Resource identity. Scope schema URL
		-- is reached through scope_id.
		resource_schema_url varchar not null default '',
		foreign key (stream_id) references metric_streams(id),
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)
