{{if .CTEs}}{{.CTEs}},{{else}}with{{end}}
		filtered as (
			select l.* {{.From}}
			where {{.Where}}
			order by {{.Order}}{{.Limit}}
		){{if arrow}}
		select l.id::varchar as logRef,
			coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as timestamp,
			l.severity_text as severityText, l.severity_number as severityNumber,
			l.event_name as eventName, l.service_name as serviceName,
			l.body
		from filtered l order by {{.Order}}{{else}},
		projected as materialized (
			select l.*, otel_value_json(l.body) as body_wire from filtered l
		)
		select cast(coalesce(to_json(list(json_object(
			'logRef',         l.id,
			'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
			'severityText',   l.severity_text,
			'severityNumber', l.severity_number,
			'eventName',      l.event_name,
			'serviceName',    l.service_name,
			'bodyPreview',    body_preview(l.body_wire),
			'bodyType',       otel_value_kind(l.body)
		) order by {{.Order}})), '[]') as varchar) as logs
		from projected l{{end}}
