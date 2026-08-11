-- attrs_mapped resolves an id array against a prebuilt attr_dict map.
--
-- Same output as attrs_json, different execution: a per-row probe into a
-- small hash map, instead of lambda unnest: lambda join: group by, which explodes
-- each owner's array into rows only to collapse it back. Measured on the
-- reference trace (4,891 spans, 2,457 events, 1,567 links), whole
-- searchSpans query:
--
-- unnest + join + group by   48-54 ms wall, 0.35 s CPU
-- attr_dict + attrs_mapped   37-40 ms wall, 0.19 s CPU
--
-- The CPU column is the one that matters for a desktop tool sharing cores
-- with the user's actual work: the grouped form buys its wall time with
-- parallelism it did not need to spend.
--
-- attrs_json stays where row counts are small -- resource_data, scope_data,
-- logs.Get, GetMetric -- since there the correlated subquery runs a handful
-- of times and needs no map built for it at all.
create or replace macro attrs_mapped(ids, m) as (
		coalesce(to_json(list_transform(
			list_sort(list_transform(ids, lambda aid: map_extract(m, aid)[1])),
			lambda e: e.j
		)), json('[]'))
	)
