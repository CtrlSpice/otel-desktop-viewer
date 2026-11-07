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
			native_body_preview(l.body) as bodyPreview, otel_value_kind(l.body) as bodyType
		from filtered l order by {{.Order}}{{else}}
		select cast(coalesce(to_json(list(json_object(
			'logRef',         l.id,
			'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
			'severityText',   l.severity_text,
			'severityNumber', l.severity_number,
			'eventName',      l.event_name,
			'serviceName',    l.service_name,
			'bodyPreview',    native_body_preview(l.body),
			'bodyType',       otel_value_kind(l.body)
		) order by {{.Order}})), '[]') as varchar) as logs
		from filtered l{{end}}
