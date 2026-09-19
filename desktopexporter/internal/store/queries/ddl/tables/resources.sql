-- id = sha256(attribute_ids, dropped_attributes_count). Attribute ids include
-- each key and canonical typed value; the sorted array makes map order
-- irrelevant without collapsing absent, empty, or differently typed values.
create table if not exists resources (
		id uuid primary key,
		seq integer not null default nextval('resource_seq'),
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)
