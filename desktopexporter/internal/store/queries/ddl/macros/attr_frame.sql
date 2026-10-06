-- attr_frame / attr_id mirror ingest.AttributeID in SQL.
--
-- The independent SQL implementation detects Go hash/encoding drift.
-- strlen() matches Go byte length; length() counts characters.
create or replace macro attr_frame(k, v) as (
		strlen(k)::varchar || ':' || k ||
		strlen(v)::varchar || ':' || v
	)
