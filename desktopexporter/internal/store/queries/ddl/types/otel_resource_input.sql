-- Appender transport only. The stored attributes are native VARIANT.
create type otel_resource_input as struct (
    id uuid, payload_id uuid, schema_url varchar,
    attributes blob, dropped_attributes_count uinteger
)
