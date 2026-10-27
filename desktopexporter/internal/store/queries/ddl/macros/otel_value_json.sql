-- UI wire encoding is a read projection, never a second stored value.
create or replace macro otel_value_json_parts(parts) as (
    otel_wire_json(parts.metadata, parts.value)::json
);
create or replace macro otel_value_json(value) as (
    otel_value_json_parts(variant_to_parquet_variant(value)::struct(metadata blob, value blob))
);
create or replace macro otel_value_kind(value) as (
    case
        when variant_typeof(value) = 'VARIANT_NULL' then 'empty'
        when variant_typeof(value) = 'VARCHAR' then 'string'
        when starts_with(variant_typeof(value), 'INT') then 'int64'
        when variant_typeof(value) = 'DOUBLE' then 'double'
        when starts_with(variant_typeof(value), 'BOOL') then 'bool'
        when variant_typeof(value) = 'BLOB' then 'bytes'
        when starts_with(variant_typeof(value), 'ARRAY') then 'array'
        when starts_with(variant_typeof(value), 'OBJECT') then 'map'
        else error('unsupported OTel VARIANT type')
    end
)
