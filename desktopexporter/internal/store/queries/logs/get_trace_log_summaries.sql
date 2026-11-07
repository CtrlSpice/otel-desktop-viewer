{{if arrow}}select l.id::varchar as logRef,
    coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as timestamp,
    span_id_wire(l.span_id) as spanID,
    l.severity_text as severityText, l.severity_number as severityNumber,
    l.service_name as serviceName, coalesce(l.event_name, '') as eventName,
    native_body_preview(l.body) as bodyPreview, otel_value_kind(l.body) as bodyType
from logs l where l.trace_id = ?::uuid
order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp), l.id{{else}}
select cast(coalesce(to_json(list(json_object(
	'logRef',         l.id,
	'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
	'spanID',         span_id_wire(l.span_id),
	'severityText',   l.severity_text,
	'severityNumber', l.severity_number,
	'serviceName',    l.service_name,
	'eventName',      coalesce(l.event_name, ''),
	'bodyPreview',    native_body_preview(l.body)
) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) asc, l.id asc)), '[]') as varchar) as logs
from logs l where l.trace_id = ?::uuid{{end}}
