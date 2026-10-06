-- Test an attribute ID computed by ingest.AttributeID without a table lookup.
-- attr_id remains an independent audit implementation, not the search path.
create or replace macro has_attr(ids, id) as (
		list_contains(ids, id)
	)
