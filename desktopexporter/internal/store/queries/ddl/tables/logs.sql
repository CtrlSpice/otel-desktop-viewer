create table if not exists logs (
		id uuid primary key,
		timestamp ubigint,
		observed_timestamp ubigint,
		trace_id uuid,
		span_id ubigint,
		severity_text varchar,
		severity_number integer,
		body variant,
		resource otel_resource not null,
		scope otel_scope not null,
		attributes variant not null,
		dropped_attributes_count uinteger,
		flags uinteger,
		event_name varchar,
		-- See the matching service_name column on spans for rationale.
		service_name varchar not null default ''
	)
