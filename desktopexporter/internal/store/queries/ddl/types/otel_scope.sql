create type otel_scope as struct (
    id uuid, name varchar, version varchar, schema_url varchar,
    attributes variant, dropped_attributes_count uinteger
)
