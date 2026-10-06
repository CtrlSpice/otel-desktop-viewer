-- uuid_list turns one bound parameter into a set of uuids.
--
-- Bind IDs as []string because the driver corrupts bound []duckdb.UUID values.
-- DuckDB parses both dashed and dashless UUID text after unnesting.
create or replace macro uuid_list(ids) as table (
    select unnest(ids::varchar[])::uuid as id
)
