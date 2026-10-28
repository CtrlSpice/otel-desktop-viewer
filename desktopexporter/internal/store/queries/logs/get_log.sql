select log_data_json(
    l, attribute_document(l.attributes),
    json_object('attributes', attribute_document(l.resource.attributes),
        'droppedAttributesCount', l.resource.dropped_attributes_count),
    json_object('name', l.scope.name, 'version', l.scope.version,
        'attributes', attribute_document(l.scope.attributes),
        'droppedAttributesCount', l.scope.dropped_attributes_count),
    l.resource.schema_url, l.scope.schema_url
)::varchar as log
from logs l where l.id = ?::uuid
