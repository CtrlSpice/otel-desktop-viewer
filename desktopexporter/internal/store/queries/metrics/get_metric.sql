-- Exact Metric identity and its retained series catalogue.
with selected_metric as materialized (
    select * from metrics where id = ?::uuid
), series_catalogue as (
    select ms.id, ms.attributes, count(d.id){{if not arrow}}::varchar{{end}} as datapoint_count,
        min(d.timestamp){{if not arrow}}::varchar{{end}} as first_datapoint_timestamp,
        max(d.timestamp){{if not arrow}}::varchar{{end}} as last_datapoint_timestamp
    from metric_series ms join selected_metric m on m.id = ms.metric_id
    left join metric_datapoints d on d.series_id = ms.id
    group by ms.id, ms.attributes
){{if arrow}}
select {{template "metricColumns"}},
    (select coalesce(list(struct_pack(seriesRef := id::varchar, attributes := attributes,
        datapointCount := datapoint_count, firstDatapointTimestamp := first_datapoint_timestamp,
        lastDatapointTimestamp := last_datapoint_timestamp) order by id), []) from series_catalogue) as series
from selected_metric m
{{else}}, series_documents as (
    select to_json(list(json_object('seriesRef', id::varchar,
        'attributes', attribute_document(attributes), 'datapointCount', datapoint_count,
        'firstDatapointTimestamp', first_datapoint_timestamp,
        'lastDatapointTimestamp', last_datapoint_timestamp) order by id)) as document
    from series_catalogue
)
select m.metric_type, json_merge_patch(json_object(
    'metricRef', m.id::varchar, 'name', m.name, 'description', m.description, 'unit', m.unit,
    'metadata', attribute_document(m.metadata), 'metricType', m.metric_type,
    'resource', json_object('attributes', attribute_document(m.resource.attributes),
        'droppedAttributesCount', m.resource.dropped_attributes_count, 'schemaUrl', m.resource.schema_url),
    'scope', json_object('name', m.scope.name, 'version', m.scope.version,
        'attributes', attribute_document(m.scope.attributes),
        'droppedAttributesCount', m.scope.dropped_attributes_count, 'schemaUrl', m.scope.schema_url),
    'series', coalesce(sd.document, '[]'::json)),
    case m.metric_type
        when 'Sum' then json_object('aggregationTemporalityCode', m.aggregation_temporality, 'isMonotonic', m.is_monotonic)
        when 'Histogram' then json_object('aggregationTemporalityCode', m.aggregation_temporality)
        when 'ExponentialHistogram' then json_object('aggregationTemporalityCode', m.aggregation_temporality)
        else '{}'::json end)::varchar as document
from selected_metric m cross join series_documents sd{{end}}
