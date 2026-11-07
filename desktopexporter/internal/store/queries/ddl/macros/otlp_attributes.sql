create or replace macro otlp_attributes(attrs) as (
    case when variant_typeof(attrs) not like 'OBJECT%' then error('OTLP attributes must be a map')
        else otlp_value(attrs)->'kvlistValue'->'values' end
)
