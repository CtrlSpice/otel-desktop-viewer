-- The attribute dictionary: one row per distinct (key, value, type, scope)
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
-- `scope` is part of identity, not a free-form tag. Attribute discovery has
-- to report attributeScope (a closed union in wire-types.ts), and scope
-- otherwise lives only in *which* array references an id -- unrecoverable
-- without unnesting every owner. The cost is that the same triple under two
-- scopes is two rows, which is negligible: the populations barely overlap.
create table if not exists attributes (
		id uuid primary key,
		key varchar not null,
		value varchar not null,
		type attr_type not null,
		scope varchar not null
	)
