-- Derived discovery/search data only. Telemetry is read from its native rows.
-- There is deliberately no token table, full-text engine or payload reconstruction.
create sequence if not exists search_owner_ids;
create sequence if not exists search_path_ids;
create sequence if not exists search_value_ids;
create table if not exists search_owners (
    id bigint primary key default nextval('search_owner_ids'),
    ref varchar unique not null, signal varchar not null, owner varchar not null,
    root_key varchar not null, record_key varchar not null,
    parent_ref varchar, resource_ref varchar, scope_ref varchar,
    fingerprint varchar not null
);
create table if not exists search_paths (
    id bigint primary key default nextval('search_path_ids'), identity varchar unique not null,
    owner varchar not null, path json not null, kind varchar not null,
    key varchar not null, key_text varchar not null,
    attribute_scope varchar not null, is_attribute boolean not null
);
create table if not exists search_values (
    id bigint primary key default nextval('search_value_ids'), identity varchar unique not null,
    kind varchar not null, value varchar
);
create table if not exists search_postings (
    owner_id bigint not null, path_id bigint not null, value_id bigint not null
);
