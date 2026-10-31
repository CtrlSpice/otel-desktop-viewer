{{if .CTEs}}{{.CTEs}},{{else}}with{{end}}
filtered as (
    select l.* {{.From}}
    where {{.Where}}
    order by {{.Order}}{{.Limit}}
),
ranked as (
    select l.id, l.severity_number,
        row_number() over (order by {{.Order}}) - 1 as row_offset
    from filtered l
),
selected as (
    select * from ranked where id = try_cast(? as uuid)
)
select cast(json_object(
    'count', (select count(*) from ranked where severity_number >= 17),
    'index', case when (select severity_number from selected) >= 17 then
        (select count(*) from ranked where severity_number >= 17 and row_offset <= (select row_offset from selected))
        else null end,
    'previous', (select json_object('logRef', id, 'offset', row_offset) from ranked
        where severity_number >= 17 and row_offset < (select row_offset from selected)
        order by row_offset desc limit 1),
    'next', (select json_object('logRef', id, 'offset', row_offset) from ranked
        where severity_number >= 17 and row_offset > coalesce((select row_offset from selected), -1)
        order by row_offset limit 1)
) as varchar)
