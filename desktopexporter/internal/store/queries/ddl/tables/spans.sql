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
		resource otel_resource not null,
		scope otel_scope not null,
		attributes variant not null,
		dropped_attributes_count uinteger,
		dropped_events_count uinteger,
		dropped_links_count uinteger,
		-- Received OTLP enum number. Labels are derived only in projections.
		status_code integer,
		status_message varchar,
		-- Derived Resource service.name for indexed search.
		service_name varchar not null default '',
		-- OTLP requires span IDs to be unique only within a trace.
		primary key (trace_id, span_id)
	)
