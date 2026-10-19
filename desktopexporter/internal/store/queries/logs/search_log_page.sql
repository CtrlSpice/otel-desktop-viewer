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
select cast(json_object(
    'total', (select count(*) from filtered),
    'logs', (select coalesce(to_json(list(json_object(
        'logRef',         l.id,
        'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
        'severityText',   l.severity_text,
        'severityNumber', l.severity_number,
        'eventName',      l.event_name,
        'serviceName',    l.service_name,
        'bodyPreview',    body_preview(l.body),
        'bodyType',       json_extract_string(l.body, '$.kind')
    ) order by {{.Order}})), '[]'::json) from page l)
) as varchar)
