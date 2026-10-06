create table if not exists spans (
		trace_id uuid,
		trace_state varchar,
		span_id ubigint not null,
		parent_span_id ubigint,
		-- W3C trace flags plus the parent-context is_remote bit.
		flags uinteger,
		name varchar,
		-- Received OTLP enum number. Labels are derived only in projections.
		kind integer,
		start_time ubigint,
		end_time ubigint,
		resource_id uuid not null,
		scope_id uuid not null,
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger,
		dropped_events_count uinteger,
		dropped_links_count uinteger,
		-- Received OTLP enum number. Labels are derived only in projections.
		status_code integer,
		status_message varchar,
		-- Derived Resource service.name for indexed search; resource_id remains
		-- authoritative. Empty string represents absence for the appender.
		service_name varchar not null default '',
		-- OTLP requires span IDs to be unique only within a trace.
		primary key (trace_id, span_id),
		foreign key (resource_id) references resources(id),
		foreign key (scope_id) references scopes(id)
	)
