-- The exact parent identity and sorted attribute list enforce deduplication;
-- the generated UUID remains only a database-local reference.
create unique index if not exists idx_metric_series_identity on metric_series (
    stream_id, (cast(attribute_ids as varchar))
)
