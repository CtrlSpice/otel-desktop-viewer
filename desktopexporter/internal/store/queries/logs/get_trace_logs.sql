select cast(coalesce(to_json(list(json_object(
	'id',             l.id,
	'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
	'spanID',         span_id_wire(l.span_id),
	'severityText',   l.severity_text,
	'severityNumber', l.severity_number,
	'serviceName',    l.service_name,
	'eventName',      coalesce(l.event_name, ''),
	'bodyPreview',    body_preview(l.body)
) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) asc, l.id asc)), '[]') as varchar) as logs
from logs l
where l.trace_id = ?::uuid
