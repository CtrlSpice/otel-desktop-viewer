{{.CTEs}},
filtered as (
    select l.* {{.From}}
    where {{.Where}}
    order by {{.Order}}{{.Limit}}
),
page as (
    select l.* from filtered l
    order by {{.Order}}
    limit ? offset ?
)
{{if .Columns}}, requested_columns as (
    select j.key::integer as position,
        j.value->>'source' as source, j.value->>'kind' as kind, j.value->>'key' as key
    from json_each(?::json) j
), projected_columns as (
    select l.id, to_json(list(json_object(
        'source', c.source, 'kind', c.kind, 'key', c.key,
        'values', log_column_values(l, r, sc, c.source, c.kind, c.key)
    ) order by c.position)) as values
    from page l
    join resources r on r.id = l.resource_id
    join scopes sc on sc.id = l.scope_id
    cross join requested_columns c
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
        'bodyPreview',    body_preview(otel_value_json(l.body)),
        'bodyType',       otel_value_kind(l.body)
        {{if .Columns}}, 'columnValues', p.values{{end}}
    ) order by {{.Order}})), '[]'::json) from page l
    {{if .Columns}}left join projected_columns p on p.id = l.id{{end}})
) as varchar)
