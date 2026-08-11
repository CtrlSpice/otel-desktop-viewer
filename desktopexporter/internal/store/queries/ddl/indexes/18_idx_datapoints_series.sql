-- Restores what idx_datapoints_stream_attrs used to do before attributes
-- became a LIST. ART indexes serve equality on a single column, which is
-- exactly the shape series grouping now has.
create index if not exists idx_datapoints_series on datapoints(series_id)
