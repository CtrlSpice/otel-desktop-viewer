{{if arrow}}select l.id::varchar as logRef,
    coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as timestamp,
    span_id_wire(l.span_id) as spanID,
    l.severity_text as severityText, l.severity_number as severityNumber,
    l.service_name as serviceName, coalesce(l.event_name, '') as eventName, l.body
from logs l where l.trace_id = ?::uuid
order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp), l.id{{else}}with projected as materialized (
	select l.*, otel_value_json(l.body) as body_wire
	from logs l where l.trace_id = ?::uuid
)
select cast(coalesce(to_json(list(json_object(
	'logRef',         l.id,
	'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
	'spanID',         span_id_wire(l.span_id),
	'severityText',   l.severity_text,
	'severityNumber', l.severity_number,
	'serviceName',    l.service_name,
	'eventName',      coalesce(l.event_name, ''),
	'bodyPreview',    body_preview(l.body_wire)
) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) asc, l.id asc)), '[]') as varchar) as logs
from projected l{{end}}
