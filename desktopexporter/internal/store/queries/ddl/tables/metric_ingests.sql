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
		resource_id uuid not null,
		scope_id uuid not null,
		-- OTLP carries a schema_url on the *batch* wrapper (ResourceSpans /
		-- ScopeSpans and their metric and log twins), not on the Resource or
		-- InstrumentationScope messages -- neither of which has the field at
		-- all. It names the semantic-convention version the batch's attributes
		-- follow, e.g. https://opentelemetry.io/schemas/1.27.0.
		--
		-- Stored per row rather than on resources/scopes precisely because it
		-- is batch-level: the same scope emitted through two pipelines that
		-- stamp different schema urls is still one scope, so putting it in
		-- scope identity would split it. Repeated varchars are what DuckDB's
		-- dictionary encoding handles well, so the duplication is close to
		-- free.
		--
		-- NOT NULL with empty-string default: the field is optional in OTLP,
		-- and the appender takes a plain string more happily than a nullable.
		resource_schema_url varchar not null default '',
		scope_schema_url varchar not null default '',
		foreign key (stream_id) references metric_streams(id),
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)
