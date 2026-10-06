-- attrs_mapped resolves an id array against a prebuilt attr_dict map.
-- Map probes avoid expanding each owner's IDs into rows before regrouping.
create or replace macro attrs_mapped(ids, m) as (
		coalesce(to_json(list_transform(
			list_sort(list_transform(ids, lambda aid: map_extract(m, aid)[1])),
			lambda e: e.j
		)), json('[]'))
	)
