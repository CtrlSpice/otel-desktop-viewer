{{.CTEs}},
		filtered as (
			select l.* {{.From}}
			where {{.Where}}
		)
		select cast(coalesce(to_json(list(json_object(
			'id',             l.id,
			'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
			'severityText',   l.severity_text,
			'severityNumber', l.severity_number,
			'serviceName',    l.service_name,
			'bodyPreview',    substring(l.body, 1, {{.BodyPreviewLen}})
		) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) desc)), '[]') as varchar) as logs
		from filtered l