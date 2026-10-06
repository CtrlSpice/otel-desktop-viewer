-- id = sha256(name, version, schema_url, attribute_ids, dropped_attributes_count).
create table if not exists scopes (
		id uuid primary key,
		seq integer not null default nextval('scope_seq'),
		name varchar not null default '',
		version varchar not null default '',
		schema_url varchar not null default '',
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)
