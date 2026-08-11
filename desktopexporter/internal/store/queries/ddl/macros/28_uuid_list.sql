-- uuid_list turns one bound parameter into a set of uuids.
--
-- Callers write a static statement:
--
--   delete from spans where span_id in (select id from uuid_list(?))
--
-- and bind the whole id set as a single []string argument. The alternative --
-- building one ?::uuid placeholder per id and appending one argument per
-- placeholder -- lets the two counts drift apart, which is a mismatch SQL
-- cannot catch because each count is individually correct.
--
-- The double cast is deliberate and is a workaround, not a preference. The
-- driver cannot bind a uuid list: []string against ?::uuid[] fails to bind at
-- all, and []duckdb.UUID binds happily and then matches nothing, because the
-- value is altered in transit (a bound 11111111-... reads back as 91111111-...,
-- with no error). Casting to varchar[] instead moves the text-to-uuid
-- conversion out of the driver and into DuckDB's own parser, which handles both
-- the dashed and the dashless wire form correctly.
--
-- util.TestDriverStillCannotBindUUIDLists pins that limitation and fails when
-- it lifts, at which point this becomes `select unnest(ids::uuid[])`.
create or replace macro uuid_list(ids) as table (
    select unnest(ids::varchar[])::uuid as id
)
