create or replace view exemplars as
select e.id, point_id as metric_datapoint_id, e.timestamp, e.double_value, e.int_value,
       e.trace_id, e.span_id, e.attributes
from (
    select d.id as point_id, unnest(d.exemplars) as e
    from (select unnest(datapoints) as d from metric_records) points
) examples;
