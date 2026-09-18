-- The attribute dictionary: one row per distinct (key, canonical JSON value)
-- for the whole database. On the reference capture, 723,692 attribute rows
-- collapse to 267 dictionary rows.
--
-- id = sha256 over the length-prefixed fields, truncated to 16 bytes.
-- Content-derived rather than surrogate, so ingest can compute it without
-- asking the database and the owners' arrays are correct by construction.
-- Identity being the primary key is also why there is no UNIQUE here: it
-- would be redundant against the PK, and would put an index over `value`,
-- which holds whole SQL statements and stack traces.
--
create table if not exists attributes (
		id uuid primary key,
		key varchar not null,
		value json not null
	)
