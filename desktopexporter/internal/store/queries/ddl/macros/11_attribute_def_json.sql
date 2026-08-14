-- One attribute *definition* -- the shape the search-field dropdowns read.
-- Identical in four places before this: two in spans.go, one each in
-- logs.go and metrics.go.
create or replace macro attribute_def_json(key, scope, type) as (
		json_object('name', key, 'attributeScope', scope, 'type', type::varchar)
	)
