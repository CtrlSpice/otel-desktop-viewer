create unique index if not exists idx_metrics_identity on metrics (
    resource_payload_id, scope_id,
    name, unit, metric_type, aggregation_temporality, is_monotonic
)
