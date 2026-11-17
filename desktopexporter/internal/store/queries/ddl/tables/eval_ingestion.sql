-- Rebuildable caches of retained, Collector-processed evaluation evidence.
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
    context variant not null
);
create table if not exists eval_records (
    import_id varchar not null,
    envelope integer not null,
    ordinal bigint not null,
    byte_size bigint not null,
    payload variant not null,
    primary key (import_id, envelope, ordinal)
);
-- Tracks physical records separately from logical source identities. Ordinary
-- OTLP logs remain append-only; repeated evidence is deduplicated in projection.
create table if not exists eval_evidence (
    log_id uuid primary key,
    import_id varchar not null
);
create table if not exists eval_transfers (
    id varchar primary key,
    dirty boolean not null default true,
    reason varchar not null default ''
);
create table if not exists eval_parts (
    import_id varchar not null,
    envelope integer not null,
    ordinal bigint not null,
    digest varchar not null,
    primary key(import_id, envelope, ordinal)
);
