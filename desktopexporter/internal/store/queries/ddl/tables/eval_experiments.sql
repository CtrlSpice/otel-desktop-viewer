create table if not exists eval_experiment_indexes (
    dataset_id varchar primary key,
    source_kind varchar not null,
    status varchar not null,
    reason varchar not null default '',
    context variant not null default '{}'::json::variant
);
create table if not exists eval_experiments (
    id varchar primary key,
    dataset_id varchar not null,
    adapter_id varchar not null,
    name varchar not null,
    name_key varchar not null,
    evaluation varchar not null,
    source varchar not null,
    format varchar not null,
    started_at varchar,
    started_sort timestamp,
    first_ordinal bigint not null,
    pass_count bigint not null,
    partial_count bigint not null,
    fail_count bigint not null,
    fault_count bigint not null,
    unscored_count bigint not null
);
create table if not exists eval_experiment_rows (
    dataset_id varchar not null,
    ordinal bigint not null,
    experiment_id varchar not null,
    primary key (dataset_id, ordinal)
);
-- Derived searchable text keeps retained evidence authoritative. One row mirrors one
-- source result, so search never requires assembling or parsing a whole experiment.
create table if not exists eval_experiment_search (
    dataset_id varchar not null,
    ordinal bigint not null,
    experiment_id varchar not null,
    text varchar not null,
    primary key (dataset_id, ordinal)
);
