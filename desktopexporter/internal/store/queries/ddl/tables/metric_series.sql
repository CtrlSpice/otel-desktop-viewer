create or replace view metric_series as
select md5(r.id::varchar||':'||d.attribute_key)::uuid as id, r.id as metric_id, d.attribute_key,
       first(d.attributes order by r.received_order, d.id) as attributes
from (select id, received_order, unnest(datapoints) as d from metric_records) r
group by r.id, d.attribute_key;
