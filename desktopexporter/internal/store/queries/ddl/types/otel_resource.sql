-- Resource payload and ResourceSpans/ResourceLogs/ResourceMetrics wrapper.
-- IDs preserve grouping identity; the complete payload lives on the signal.
create type otel_resource as struct (
    id uuid, payload_id uuid, schema_url varchar,
    attributes variant, dropped_attributes_count uinteger
)
