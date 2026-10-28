-- UI wire encoding is a read projection, never a second stored value.
-- Native SQL objects need not be sorted; Parquet VARIANT requires sorted fields.
create or replace macro otel_value_json_parts(parts) as (
    otel_wire_json(parts.metadata, parts.value)::json
);
create or replace macro otel_value_json(value) as (
    otel_value_json_parts(variant_to_parquet_variant(variant_normalize(value))::struct(metadata blob, value blob))
);
-- Labels follow pdata's AsString without an intermediate tagged JSON tree.
create or replace macro otel_value_text_parts(parts) as (
    otel_native_text(parts.metadata, parts.value)
);
create or replace macro otel_value_text(value) as (
    otel_value_text_parts(variant_to_parquet_variant(variant_normalize(value))::struct(metadata blob, value blob))
);
-- Search readable native values, never the wire encoder's kind/value envelope.
-- Explicit empty values remain distinguishable from absent SQL NULL values.
create or replace macro otel_search_text(value) as (
    case when variant_typeof(value) = 'VARIANT_NULL' then 'null'
         when value is null then null
         else otel_value_text(value) end
);
create or replace macro otel_type_kind(type_name) as (
    case
        when type_name = 'VARIANT_NULL' then 'empty'
        when type_name = 'VARCHAR' then 'string'
        when starts_with(type_name, 'INT') then 'int64'
        when type_name = 'DOUBLE' then 'double'
        when starts_with(type_name, 'BOOL') then 'bool'
        when type_name = 'BLOB' then 'bytes'
        when starts_with(type_name, 'ARRAY') then 'array'
        when starts_with(type_name, 'OBJECT') then 'map'
        else error('unsupported OTel VARIANT type')
    end
);
-- Keep selected-field null/empty-key behavior. Discovery classifies complete
-- native child values with variant_type instead of formatting nested types.
create or replace macro otel_value_kind(value) as (
    otel_type_kind(variant_typeof(value))
)
