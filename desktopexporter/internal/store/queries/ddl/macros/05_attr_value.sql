-- attr_value looks one attribute up by key, NULL when absent.
--
-- This is how resource.* and scope.* attribute *searches* resolve, now that
-- there is no per-owner attributes row to correlate against: the array is
-- already on the joined resources / scopes row, and that table is a couple
-- of dozen rows, so the lookup is effectively cached.
--
-- Note it is NOT how service.name is read on the hot filter path.
-- spans.service_name and logs.service_name were kept as denormalized
-- columns (the plan had proposed dropping them); a column scan still beats
-- a join plus an unnest for the single most-filtered-on field. This macro
-- is what verifies those columns agree with the resource they came from.
create or replace macro attr_value(ids, k) as (
		(select a.value
		 from unnest(ids) as t(aid)
		 join attributes a on a.id = t.aid
		 where a.key = k
		 limit 1)
	)
