-- Query-appender transport, not persisted. A complete tree is one LIST cell, so
-- automatic appender flushes cannot split a value. Root id is 0, parent is -1.
create type otel_value_node as struct (
    id bigint, parent bigint, ordinal bigint, key varchar, kind varchar,
    children bigint, string_value varchar, int_value bigint, double_value double,
    bool_value boolean, bytes_value blob
)
