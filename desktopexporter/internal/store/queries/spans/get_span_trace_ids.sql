select cast(coalesce(to_json(list(trace_id_wire(trace_id) order by trace_id)), '[]') as varchar)
from spans
where span_id = (select unnest(?::ubigint[]))
