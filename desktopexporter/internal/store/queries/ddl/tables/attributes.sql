-- One row per distinct (key, canonical JSON value). id is SHA-256 over
-- length-prefixed fields, truncated to 16 bytes.
create table if not exists attributes (
		id uuid primary key,
		key varchar not null,
		value json not null
	)
