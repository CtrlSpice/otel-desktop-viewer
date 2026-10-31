create or replace view events as
select e.id, s.trace_id, s.span_id, e.name, e.timestamp, e.attributes, e.dropped_attributes_count, e.ordinal
from (select trace_id, span_id, unnest(events) as e from spans) s;
