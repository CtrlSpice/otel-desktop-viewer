{{if .CTEs}}{{.CTEs}},{{else}}with{{end}}
filtered as (
    select l.* {{.From}}
    where {{.Where}}
    order by {{.Order}}{{.Limit}}
),
page as (
    select l.* from filtered l
    order by {{.Order}}
    limit ? offset ?
), page_values as materialized (
    select l.*, otel_value_json(l.body) as body_wire from page l
)
{{if .Columns}}, requested_columns as (
    select j.key::integer as position,
        j.value->>'source' as source, j.value->>'kind' as kind, j.value->>'key' as key
    from json_each(?::json) j
), column_attributes as materialized (
    select l.id, c.position,
        coalesce((select json_array(entry.value->'value')
            from json_each(otel_value_json(attrs), '$.value') entry
            where entry.value->>'key' = c.key), '[]'::json) as values
    from page l
    cross join requested_columns c
    cross join lateral (select case c.source when 'log' then l.attributes
        when 'resource' then l.resource.attributes when 'scope' then l.scope.attributes end as attrs)
    where c.kind = 'attribute'
), column_fields as materialized (
    select l.id, c.position, log_column_values(l, l.resource, l.scope, c.source, c.key, l.body_wire) as values
    from page_values l
    cross join requested_columns c
    where c.kind = 'field'
), projected_columns as (
    select l.id, to_json(list(json_object(
        'source', c.source, 'kind', c.kind, 'key', c.key,
        'values', coalesce(case when c.kind = 'attribute' then a.values else f.values end, '[]'::json)
    ) order by c.position)) as values
    from page_values l
    cross join requested_columns c
    left join column_attributes a on a.id = l.id and a.position = c.position
    left join column_fields f on f.id = l.id and f.position = c.position
    group by l.id
)
{{end}}
select cast(json_object(
    'total', (select count(*) from filtered),
    'logs', (select coalesce(to_json(list(json_object(
        'logRef',         l.id,
        'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
        'severityText',   l.severity_text,
        'severityNumber', l.severity_number,
        'eventName',      l.event_name,
        'serviceName',    l.service_name,
        'bodyPreview',    body_preview(l.body_wire),
        'bodyType',       otel_value_kind(l.body)
        {{if .Columns}}, 'columnValues', p.values{{end}}
    ) order by {{.Order}})), '[]'::json) from page_values l
    {{if .Columns}}left join projected_columns p on p.id = l.id{{end}})
) as varchar)
