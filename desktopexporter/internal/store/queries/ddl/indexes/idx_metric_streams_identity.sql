create unique index if not exists idx_metric_streams_identity on metric_streams (
    resource_payload_id, scope_id,
    name, unit, metric_type, aggregation_temporality, is_monotonic
)
