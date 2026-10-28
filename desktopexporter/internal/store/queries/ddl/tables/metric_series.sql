-- One chart series per exact Metric ID and datapoint attribute set. id is a
-- database-local stable reference.
create table if not exists metric_series (
		id uuid primary key,
		metric_id uuid not null,
		attribute_key varchar not null,
		attributes variant not null
	)
