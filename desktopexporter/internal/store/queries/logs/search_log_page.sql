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
){{if .Columns}}, page_columns as (
    select l.*, [{{.NativeColumns}}] as column_values from page l
){{end}}{{if arrow}}
select (select count(*) from filtered) as total,
    (select coalesce(list(struct_pack(
        logRef := l.id::varchar,
        timestamp := coalesce(nullif(l.timestamp, 0), l.observed_timestamp),
        severityText := l.severity_text, severityNumber := l.severity_number,
        eventName := l.event_name, serviceName := l.service_name,
        bodyPreview := native_body_preview(l.body), bodyType := otel_value_kind(l.body)
        {{if .Columns}}, columnValues := l.column_values{{end}}
    ) order by {{.Order}}), []) from {{if .Columns}}page_columns{{else}}page{{end}} l
    ) as logs
{{else}}
select cast(json_object(
    'total', (select count(*) from filtered),
    'logs', (select coalesce(to_json(list(json_object(
        'logRef',         l.id,
        'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
        'severityText',   l.severity_text,
        'severityNumber', l.severity_number,
        'eventName',      l.event_name,
        'serviceName',    l.service_name,
        'bodyPreview',    native_body_preview(l.body),
        'bodyType',       otel_value_kind(l.body)
        {{if .Columns}}, 'columnValues', l.column_values{{end}}
    ) order by {{.Order}})), '[]'::json) from {{if .Columns}}page_columns{{else}}page{{end}} l)
) as varchar){{end}}
