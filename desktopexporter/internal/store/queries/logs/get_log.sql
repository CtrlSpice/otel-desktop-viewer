
		select cast(json_object(
			'id', l.id,
			'timestamp', l.timestamp::varchar,
			'observedTimestamp', l.observed_timestamp::varchar,
			'traceID', trace_id_wire(l.trace_id),
			'spanID', span_id_wire(l.span_id),
			'severityText', l.severity_text,
			'severityNumber', l.severity_number,
			'body', l.body,
			'bodyType', l.body_type,
			'resource', resource_json(r.attribute_ids, r.dropped_attributes_count),
			'scope', scope_json(sc.name, sc.version, sc.attribute_ids, sc.dropped_attributes_count),
			'droppedAttributesCount', l.dropped_attributes_count,
			'flags', l.flags,
			'eventName', l.event_name,
			'attributes', attrs_json(l.attribute_ids)
		) as varchar) as log
		from logs l
		join resources r on r.id = l.resource_id
		join scopes sc on sc.id = l.scope_id
		where l.id = ?::uuid
	