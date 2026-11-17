create type otel_scope_input as struct (
    id uuid, name varchar, version varchar, schema_url varchar,
    attributes blob, dropped_attributes_count uinteger
)
