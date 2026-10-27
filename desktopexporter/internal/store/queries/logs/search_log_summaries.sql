{{.CTEs}},
		filtered as (
			select l.* {{.From}}
			where {{.Where}}
			order by {{.Order}}{{.Limit}}
		)
		select cast(coalesce(to_json(list(json_object(
			'logRef',         l.id,
			'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
			'severityText',   l.severity_text,
			'severityNumber', l.severity_number,
			'eventName',      l.event_name,
			'serviceName',    l.service_name,
			'bodyPreview',    body_preview(otel_value_json(l.body)),
			'bodyType',       otel_value_kind(l.body)
		) order by {{.Order}})), '[]') as varchar) as logs
		from filtered l
