create or replace view metric_datapoints as
select d.id, r.id as metric_id, md5(r.id::varchar||':'||d.attribute_key)::uuid as series_id,
       d.timestamp, d.start_time, d.flags, d.double_value, d.int_value, d.value_type,
       d.count, d.sum, d.min, d.max, d.bucket_counts, d.explicit_bounds,
       d.scale, d.zero_count, d.zero_threshold, d.positive_bucket_offset, d.positive_bucket_counts,
       d.negative_bucket_offset, d.negative_bucket_counts, d.attributes, r.resource, r.scope
from (select id, resource, scope, unnest(datapoints) as d from metric_records) r;

create or replace macro metric_points_for(metric_id_, time_start_, time_end_) as table
select d.id, r.id as metric_id, md5(r.id::varchar||':'||d.attribute_key)::uuid as series_id,
       d.timestamp, d.start_time, d.flags, d.double_value, d.int_value, d.value_type,
       d.count, d.sum, d.min, d.max, d.bucket_counts, d.explicit_bounds,
       d.scale, d.zero_count, d.zero_threshold, d.positive_bucket_offset, d.positive_bucket_counts,
       d.negative_bucket_offset, d.negative_bucket_counts, d.attributes, r.resource, r.scope, d.exemplars
from (
    select id, resource, scope, unnest(list_filter(datapoints, lambda p:
        (time_start_ is null or p.timestamp>=time_start_) and
        (time_end_ is null or p.timestamp<=time_end_))) as d
    from metric_records where id=metric_id_
    and (time_start_ is null or point_time_max>=time_start_)
    and (time_end_ is null or point_time_min<=time_end_)
) r;
