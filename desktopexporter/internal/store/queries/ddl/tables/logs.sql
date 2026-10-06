create table if not exists logs (
		id uuid primary key,
		timestamp ubigint,
		observed_timestamp ubigint,
		trace_id uuid,
		span_id ubigint,
		severity_text varchar,
		severity_number integer,
		body json,
		resource_id uuid not null,
		scope_id uuid not null,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		flags uinteger,
		event_name varchar,
		-- See the matching service_name column on spans for rationale.
		service_name varchar not null default '',
		-- Resource schema URL belongs to the ResourceLogs wrapper and is not
		-- part of Resource identity. Scope schema URL is reached through
		-- scope_id because it participates in instrumentation Scope identity.
		resource_schema_url varchar not null default '',
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)
