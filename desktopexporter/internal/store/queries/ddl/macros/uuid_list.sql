-- uuid_list turns one bound parameter into a set of uuids.
--
-- Bind API IDs as []string and convert from varchar[] to UUIDs in SQL.
-- DuckDB parses both dashed and dashless UUID text after unnesting.
create or replace macro uuid_list(ids) as table (
    select unnest(ids::varchar[])::uuid as id
)
