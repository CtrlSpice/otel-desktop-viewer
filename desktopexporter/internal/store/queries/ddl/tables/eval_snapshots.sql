-- JSON is stored by direct cast, never json() or a parse/re-encode operation.
-- CAST(payload AS VARCHAR) recovers the original text; fidelity is pinned by
-- file-backed reopen tests, including whitespace, numeric lexemes and keys.
create table if not exists eval_snapshots (
    id varchar primary key,
    source varchar not null,
    format varchar not null,
    name varchar not null,
    imported_at timestamp not null default current_timestamp,
    byte_size bigint not null,
    payload json not null
);
