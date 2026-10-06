-- payload_id = sha256(sorted attribute_ids, dropped_attributes_count).
-- id = sha256(payload_id, schema_url), identifying the exact Resource wrapper.
create table if not exists resources (
		id uuid primary key,
		payload_id uuid not null,
		seq integer not null default nextval('resource_seq'),
		schema_url varchar not null default '',
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0,
		unique (id, payload_id)
	)
