-- Additive, self-contained received-data storage. Incomplete imports are hidden.
create table if not exists eval_imports (
    id varchar primary key,
    complete boolean not null default false
);
create table if not exists eval_datasets (
    id varchar primary key,
    import_id varchar not null,
    envelope integer not null,
    source varchar not null,
    format varchar not null,
    name varchar not null,
    imported_at timestamp not null default current_timestamp,
    byte_size bigint not null,
    row_count bigint not null,
    deleted boolean not null default false,
    context json not null
);
create table if not exists eval_records (
    import_id varchar not null,
    envelope integer not null,
    ordinal bigint not null,
    byte_size bigint not null,
    payload json not null,
    primary key (import_id, envelope, ordinal)
);
