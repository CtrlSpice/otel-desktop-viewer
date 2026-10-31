{{if arrow}}select {{template "logColumns"}}{{else}}select coalesce(to_json(list(log_data_json(
    l, attribute_document(l.attributes),
    json_object('attributes', attribute_document(l.resource.attributes),
        'droppedAttributesCount', l.resource.dropped_attributes_count),
    json_object('name', l.scope.name, 'version', l.scope.version,
        'attributes', attribute_document(l.scope.attributes),
        'droppedAttributesCount', l.scope.dropped_attributes_count),
    l.resource.schema_url, l.scope.schema_url
) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp), l.id)), '[]')::varchar as logs{{end}}
from logs l where l.trace_id = ?::uuid and l.span_id = (select unnest(?::ubigint[]))
{{if arrow}}order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp), l.id{{end}}
