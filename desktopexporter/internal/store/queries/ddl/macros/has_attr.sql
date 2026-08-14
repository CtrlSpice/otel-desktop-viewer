-- has_attr tests membership by id. An inline array probe -- no join, no
-- correlated subquery into a table of owner-attribute rows.
--
-- Callers pass an id computed in Go by ingest.AttributeID, which is why
-- exact-match attribute search needs no database round trip to build its
-- predicate. Deliberately not attr_id(...) inline: that would put the audit
-- macro on the correctness path, where a Go/SQL divergence turns into search
-- quietly returning nothing instead of a failing test.
create or replace macro has_attr(ids, id) as (
		list_contains(ids, id)
	)
