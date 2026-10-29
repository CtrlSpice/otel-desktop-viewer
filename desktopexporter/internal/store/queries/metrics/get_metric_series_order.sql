-- Prune owning records by metric and timestamp bounds before expanding points.
-- Filter exact timestamps before cohort grouping, label preparation or sorting.
with scoped_points as materialized (
    select r.id as metric_id, r.received_order,
        unnest(list_filter(r.datapoints, lambda p:
            (list_extract($2::ubigint[], 1) is null or p.timestamp >= list_extract($2::ubigint[], 1)) and
            (list_extract($3::ubigint[], 1) is null or p.timestamp <= list_extract($3::ubigint[], 1)))) as d
    from metric_records r
    where r.id = $1::uuid
      and (list_extract($2::ubigint[], 1) is null or r.point_time_max >= list_extract($2::ubigint[], 1))
      and (list_extract($3::ubigint[], 1) is null or r.point_time_min <= list_extract($3::ubigint[], 1))
), requested as (
    select distinct unnest($4::varchar[]) as id
), selected_series as materialized (
    select q.id, first(p.d.attributes order by p.received_order, p.d.id) as attributes
    from scoped_points p join requested q
      on q.id = md5(p.metric_id::varchar || ':' || p.d.attribute_key)::uuid::varchar
    group by q.id
), column_parts as materialized (
    select s.id, variant_to_parquet_variant(attribute_native_field(
        variant_to_parquet_variant(variant_normalize(s.attributes))::struct(metadata blob, value blob),
        $5::varchar))::struct(metadata blob, value blob) as parts
    from selected_series s
    where list_contains(variant_keys(s.attributes), $5::varchar)
), value_info as materialized (
    select parts, otel_metric_sort_info(parts.metadata, parts.value) as info
    from (select distinct parts from column_parts)
), cells as (
    select s.id, coalesce(v.info.label, '[absent]') as label,
        coalesce(v.info.signature, '[]') as signature,
        coalesce(v.info.kind, 'missing') as kind, v.info.numericKey as numeric_key
    from selected_series s left join column_parts c using (id)
    left join value_info v using (parts)
), displays as materialized (
    select *, case
        when count(distinct signature) over (partition by label) <= 1 then label
        when count(distinct signature) over (partition by label, kind) <= 1 then label || ' (' || kind || ')'
        else signature end as display
    from cells
), text_keys as materialized (
    select display, signature, otel_metric_text_sort_key(display, true) as display_key,
        otel_metric_text_sort_key(signature, false) as signature_key
    from (select distinct display, signature from displays)
), ordered as (
    select id as seriesRef, row_number() over (order by
        (numeric_key is null) asc,
        numeric_key {{if .Descending}}desc{{else}}asc{{end}},
        display_key {{if .Descending}}desc{{else}}asc{{end}},
        signature_key {{if .Descending}}desc{{else}}asc{{end}},
        id asc) as position
    from displays join text_keys using (display, signature)
)
{{if arrow}}
select seriesRef from ordered order by position
{{else}}
select coalesce(to_json(list(seriesRef order by position)), '[]'::json)::varchar from ordered
{{end}}
