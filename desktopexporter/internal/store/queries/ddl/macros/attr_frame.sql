-- attr_frame / attr_id mirror ingest.AttributeID in SQL.
--
-- This is a deliberate second implementation, not shared code. A Go-side
-- re-hash would use the very function that wrote the ids and could only
-- ever catch storage corruption; an independent one also catches a bug in
-- the Go hashing, and catches the encoding drifting between builds -- the
-- failure mode that is far likelier than a 128-bit collision.
--
-- Two traps, both found by testing rather than reading docs:
-- - strlen() is byte length and matches Go's len(); length() counts
-- characters and would diverge on any non-ASCII value.
-- - k::blob is not a usable way to get byte length: DuckDB rejects
-- non-ASCII in a VARCHAR->BLOB cast.
--
-- Verified against an independent shasum on ASCII and UTF-8 input.
create or replace macro attr_frame(k, v, t, s) as (
		strlen(k)::varchar || ':' || k ||
		strlen(v)::varchar || ':' || v ||
		strlen(t)::varchar || ':' || t ||
		strlen(s)::varchar || ':' || s
	)
