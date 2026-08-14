create table if not exists logs (
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
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)
