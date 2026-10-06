-- DuckDB cannot index UUID lists directly. Their VARCHAR representation keeps
-- every UUID in the sorted list; it is an exact key, not a hash or an ID.
create unique index if not exists idx_metric_streams_identity on metric_streams (
    (cast(resource_attribute_ids as varchar)),
    scope_name, scope_version, scope_schema_url,
    (cast(scope_attribute_ids as varchar)),
    name, unit, metric_type, aggregation_temporality, is_monotonic
)
