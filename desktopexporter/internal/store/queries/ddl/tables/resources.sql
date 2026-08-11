-- id = sha256(attribute_ids, dropped_attributes_count).
--
-- dropped_attributes_count is part of identity, so a resource with the same
-- attributes but a different dropped count is a separate row. Correct, but
-- note an exporter that varies its dropped count fragments the dedupe.
create table if not exists resources (
		id uuid primary key,
		seq integer not null default nextval('resource_seq'),
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)
