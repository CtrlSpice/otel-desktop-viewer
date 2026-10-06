-- One chart series per exact Metric ID and datapoint attribute set. id is a
-- database-local stable reference.
create table if not exists metric_series (
		id uuid primary key,
		metric_id uuid not null,
		attribute_ids uuid[] not null
	)
