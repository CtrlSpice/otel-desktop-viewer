-- Support equality grouping by series ID.
create index if not exists idx_metric_datapoints_series on metric_datapoints(series_id)
