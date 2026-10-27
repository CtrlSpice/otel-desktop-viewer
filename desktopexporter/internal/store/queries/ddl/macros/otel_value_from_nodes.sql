-- Scalars arrive in typed appender columns, never inferred from JSON text.
create or replace macro otel_value_leaf(n) as (
    case n.kind
        when 'empty' then 'null'::json::variant
        when 'string' then n.string_value::variant
        when 'int64' then n.int_value::variant
        when 'double' then n.double_value::variant
        when 'bool' then n.bool_value::variant
        when 'bytes' then n.bytes_value::variant
        when 'array' then []::variant[]::variant
        when 'map' then '{}'::json::variant
        else error('unknown OTel value kind')
    end
);

-- Assemble containers bottom-up. Completed children remain available when
-- siblings have different depths. Map keys and array order come from pdata.
create or replace macro otel_value_tree(input_nodes) as (
    with recursive
    nodes as materialized (select unnest(input_nodes, recursive := true)),
    built(id, value) using key(id) as (
        select id, otel_value_leaf(n)
        from nodes n where children = 0
        union
        select p.id, case p.kind
            when 'array' then list(c.value order by n.ordinal)::variant
            else variant_group_object(n.key, c.value order by n.ordinal)
                filter (where p.kind = 'map')
        end
        from nodes p
        join nodes n on n.parent = p.id
        join recurring.built c on c.id = n.id
        where not exists (select 1 from recurring.built done where done.id = p.id)
        group by p.id, p.kind, p.children
        having count(*) = p.children
    )
    select value from built where id = 0
);

create or replace macro otel_value_from_nodes(input_nodes) as (
    case when len(input_nodes) = 1 then otel_value_leaf(input_nodes[1])
        else otel_value_tree(input_nodes) end
)
