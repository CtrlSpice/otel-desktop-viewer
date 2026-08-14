create table if not exists spans (
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
