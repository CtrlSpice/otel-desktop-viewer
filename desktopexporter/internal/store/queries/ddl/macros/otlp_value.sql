-- Direct typed-value export. SQL NULL is missing data, not an empty AnyValue.
create or replace macro otlp_value_parts(parts) as (
    otel_otlp_json(parts.metadata, parts.value)::json
);
create or replace macro otlp_value(value) as (
    case when value is null then error('OTLP value is SQL NULL')
        else otlp_value_parts(variant_to_parquet_variant(variant_normalize(value))::struct(metadata blob, value blob)) end
);
