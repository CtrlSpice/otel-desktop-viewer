create or replace macro scope_json(name, version, ids, dropped) as (
		json_object('name', name, 'version', version,
		            'attributes', attrs_json(ids), 'droppedAttributesCount', dropped)
	)
