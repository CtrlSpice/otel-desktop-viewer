-- Walk native values; key/type-only discovery needs no JSON payload encoding.
create or replace macro attribute_native_field(parts, key) as (
    variant_bytes_to_variant(otel_native_object_field(parts.metadata, parts.value, key))
);

create or replace macro attribute_native_entries(attrs) as table (
    with recursive nodes(path, value) as (
        select []::json[], attrs::variant
        union all
        select list_append(nodes.path, child.part), child.value
        from nodes, lateral (
            select to_json(key) as part,
                attribute_native_field(
                    variant_to_parquet_variant(variant_normalize(nodes.value))
                        ::struct(metadata blob, value blob), key) as value
            from unnest(case when variant_type(nodes.value) = 'OBJECT'
                then variant_keys(nodes.value) else []::varchar[] end) keys(key)
            union all
            -- List casts expose explicit VARIANT nulls as SQL NULLs. Every
            -- enumerated slot exists, so restore the native empty value.
            -- Empty Parquet metadata followed by the native null value.
            select to_json(idx - 1), coalesce(value,
                variant_bytes_to_variant('\x01\x00\x00\x00'::blob))
            from unnest(case when variant_type(nodes.value) = 'ARRAY'
                then nodes.value::variant[] else []::variant[] end)
                with ordinality elements(value, idx)
        ) child
    )
    select path,
        case when len(path) = 1 then path[1]->>'$'
             else to_json(path)::varchar end as key,
        value as native_value, otel_type_kind(variant_type(value)) as type
    from nodes where len(path) > 0
);

-- Preserve the UI projection for callers that request actual values.
create or replace macro attribute_entries(attrs) as table (
    select path, key, otel_value_json(native_value) as value, type
    from attribute_native_entries(attrs)
);
