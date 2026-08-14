-- Component objects. resource_json / scope_json take the row's own fields
-- rather than an id, so a caller that already joined the row needs no
-- second lookup.
create or replace macro resource_json(ids, dropped) as (
		json_object('attributes', attrs_json(ids), 'droppedAttributesCount', dropped)
	)
