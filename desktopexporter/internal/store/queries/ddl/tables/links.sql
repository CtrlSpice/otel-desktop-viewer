create or replace view links as
select l.id, s.trace_id, s.span_id, l.linked_trace_id, l.linked_span_id, l.trace_state,
       l.attributes, l.dropped_attributes_count, l.flags, l.ordinal
from (select trace_id, span_id, unnest(links) as l from spans) s;
