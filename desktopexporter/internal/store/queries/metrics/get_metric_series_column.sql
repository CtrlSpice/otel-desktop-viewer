-- One exact comparison column for an explicit displayed cohort. Series IDs
-- include the complete typed attribute identity; no time-window reconstruction.
with selected_series as materialized (
    select md5(r.id::varchar || ':' || d.attribute_key)::uuid as id,
        first(d.attributes order by r.received_order, d.id) as attributes
    from (select id, received_order, unnest(datapoints) as d
          from metric_records where id = $1::uuid) r
    where list_contains($2::varchar[], md5(r.id::varchar || ':' || d.attribute_key)::uuid::varchar)
    group by r.id, d.attribute_key
), column_values as (
    select s.id, variant_group_object(keys.key,
        attribute_native_field(parts.value, keys.key)) as attributes
    from selected_series s,
        lateral (select variant_to_parquet_variant(variant_normalize(s.attributes))
            ::struct(metadata blob, value blob) as value) parts,
        unnest(variant_keys(s.attributes)) keys(key)
    where keys.key = $3::varchar
    group by s.id
), result as (
    select s.id::varchar as seriesRef,
        coalesce(c.attributes, '{}'::json::variant) as attributes
    from selected_series s left join column_values c using (id)
)
{{if arrow}}
select * from result order by seriesRef
{{else}}
select coalesce(to_json(list(json_object('seriesRef', seriesRef,
    'attributes', attribute_document(attributes)) order by seriesRef)), '[]'::json)::varchar
from result
{{end}}
