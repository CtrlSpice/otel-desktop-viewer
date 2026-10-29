{{define "resource"}}struct_pack(attributes := {{.}}.attributes,
    droppedAttributesCount := {{.}}.dropped_attributes_count, schemaUrl := {{.}}.schema_url){{end}}
{{define "scope"}}struct_pack(name := {{.}}.name, version := {{.}}.version,
    attributes := {{.}}.attributes, droppedAttributesCount := {{.}}.dropped_attributes_count,
    schemaUrl := {{.}}.schema_url){{end}}
{{define "logColumns"}}l.id::varchar as logRef,
    l.timestamp as timestamp, l.observed_timestamp as observedTimestamp,
    trace_id_wire(l.trace_id) as traceID, span_id_wire(l.span_id) as spanID,
    l.severity_number as severityNumber, l.severity_text as severityText,
    l.event_name as eventName, l.service_name as serviceName,
    l.flags, l.body, l.attributes, l.dropped_attributes_count as droppedAttributesCount,
    {{template "resource" "l.resource"}} as resource,
    {{template "scope" "l.scope"}} as scope{{end}}
{{define "traceNative"}}
event_data as (
    select e.trace_id, e.span_id, list(struct_pack(name := e.name, timestamp := e.timestamp,
        droppedAttributesCount := e.dropped_attributes_count, attributes := e.attributes)
        order by e.timestamp, e.ordinal) as events
    from events e where exists (select 1 from tree t where t.trace_id = e.trace_id and t.span_id = e.span_id)
    group by e.trace_id, e.span_id
), link_data as (
    select l.trace_id, l.span_id, list(struct_pack(
        traceID := trace_id_wire(l.linked_trace_id), spanID := span_id_wire(l.linked_span_id),
        traceState := l.trace_state, flags := l.flags, attributes := l.attributes,
        droppedAttributesCount := l.dropped_attributes_count,
        summary := struct_pack(target := case when target.span_id is not null then struct_pack(
            name := coalesce(target.name, ''), serviceName := target.service_name) end)) order by l.ordinal) as links
    from links l left join spans target on target.trace_id = l.linked_trace_id and target.span_id = l.linked_span_id
    where exists (select 1 from tree t where t.trace_id = l.trace_id and t.span_id = l.span_id)
    group by l.trace_id, l.span_id
), resource_data as (
    select resource.id as id, row_number() over (order by resource.id) as seq,
        {{template "resource" "resource"}} as obj
    from (select distinct resource from tree)
), scope_data as (
    select scope.id as id, row_number() over (order by scope.id) as seq,
        {{template "scope" "scope"}} as obj
    from (select distinct scope from tree)
), ordered_spans as (
    select struct_pack(spanData := struct_pack(
        traceState := ts.trace_state, spanID := span_id_wire(ts.span_id), parentSpanID := span_id_wire(ts.parent_span_id),
        flags := ts.flags, name := ts.name, kindCode := ts.kind,
        startTime := ts.start_time, endTime := ts.end_time,
        attributes := ts.attributes, events := coalesce(ed.events, []), links := coalesce(ld.links, []),
        r := rd.seq, s := scd.seq,
        droppedAttributesCount := ts.dropped_attributes_count,
        droppedEventsCount := ts.dropped_events_count, droppedLinksCount := ts.dropped_links_count,
        statusCodeValue := ts.status_code, statusMessage := ts.status_message),
        depth := ts.depth, matched := {{.MatchedExpr}}
        {{if .Salvage}}, salvaged := ts.salvaged, cyclePoint := ts.cycle_point{{end}}) as node,
        ts.sort_path
    from tree ts
    join resource_data rd on rd.id = ts.resource.id
    join scope_data scd on scd.id = ts.scope.id
    {{.MatchedJoin}}
    left join event_data ed on ed.trace_id = ts.trace_id and ed.span_id = ts.span_id
    left join link_data ld on ld.trace_id = ts.trace_id and ld.span_id = ts.span_id
)
select trace_id_wire(try_cast($1 as uuid)) as traceID,
    (select coalesce(min(start_time), 0) from tree) as traceStart,
    (select coalesce(list(obj order by seq), []) from resource_data) as resources,
    (select coalesce(list(obj order by seq), []) from scope_data) as scopes,
    (select count(*) from trace_spans) - (select count(*) from
        {{if .Selected}}{{if .Salvage}}walked{{else}}spans_tree{{end}}{{else}}tree{{end}}) as unplacedSpanCount,
    (select coalesce(list(node order by sort_path), []) from ordered_spans) as spans
where exists (select 1 from trace_spans)
{{end}}
{{define "metricColumns"}}m.id::varchar as metricRef, m.name, m.description, m.unit,
    m.metadata, m.metric_type as metricType,
    case when m.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then m.aggregation_temporality end as aggregationTemporalityCode,
    case when m.metric_type = 'Sum' then m.is_monotonic end as isMonotonic,
    {{template "resource" "m.resource"}} as resource, {{template "scope" "m.scope"}} as scope{{end}}
{{define "exemplar"}}struct_pack(timestamp := e.timestamp,
    valueType := case when e.double_value is not null then 'Double' when e.int_value is not null then 'Int' else 'Empty' end,
    doubleValue := e.double_value, intValue := e.int_value,
    traceID := trace_id_wire(e.trace_id), spanID := span_id_wire(e.span_id), filteredAttributes := e.attributes){{end}}
{{define "receivedPoint"}}struct_pack(datapointRef := d.id::varchar,
    timestamp := d.timestamp, startTime := d.start_time, flags := d.flags,
    exemplars := coalesce(e.documents, []), attributes := d.attributes,
    resource := {{template "resource" "d.resource"}}, scope := {{template "scope" "d.scope"}},
    valueType := d.value_type, doubleValue := d.double_value, intValue := d.int_value,
    count := d.count, sum := d.sum, min := d.min, max := d.max,
    bucketCounts := d.bucket_counts, explicitBounds := d.explicit_bounds,
    scale := d.scale, zeroCount := d.zero_count, zeroThreshold := d.zero_threshold,
    positive := struct_pack("offset" := d.positive_bucket_offset, bucketCounts := d.positive_bucket_counts),
    negative := struct_pack("offset" := d.negative_bucket_offset, bucketCounts := d.negative_bucket_counts)){{end}}
{{define "scalarBucket"}}struct_pack(bucketStart := a.bucket_start, sampleCount := a.sample_count,
    sum := a.value_sum, avg := a.value_avg, rate := a.rate, slope := sl.slope, hasReset := a.has_reset){{end}}
{{define "viewPoint"}}struct_pack(id := d.id::varchar, metricType := d.metric_type,
    timestamp := d.timestamp, startTime := d.start_time, flags := d.flags,
    exemplars := coalesce(ea.exemplars, []),
    exemplarCount := case when ea.exemplar_count > len(coalesce(ea.exemplars, [])) then ea.exemplar_count end,
    valueType := d.value_type, doubleValue := d.double_value, intValue := d.int_value,
    attributes := d.attributes,
    count := d.count, sum := d.sum, min := d.min, max := d.max,
    bucketCounts := d.bucket_counts, explicitBounds := d.explicit_bounds,
    scale := d.scale, zeroCount := d.zero_count, zeroThreshold := d.zero_threshold,
    positiveBucketOffset := d.positive_bucket_offset, positiveBucketCounts := d.positive_bucket_counts,
    negativeBucketOffset := d.negative_bucket_offset, negativeBucketCounts := d.negative_bucket_counts,
    aggregationTemporalityCode := d.aggregation_temporality, isMonotonic := d.is_monotonic,
    deltaInt := case when d.aggregation_temporality = 2 then d.delta_int end,
    deltaDouble := case when d.aggregation_temporality = 2 then d.delta_double end,
    isReset := case when d.aggregation_temporality = 2 then d.is_reset end,
    quantiles := {{if .NoHistogramMerge}}null{{else}}dq.quantiles{{end}}){{end}}
{{define "viewSeries"}}struct_pack(seriesRef := t.attrs_key, attributes := t.attributes,
    resource := s.resource_json, datapoints := coalesce(tj.datapoints, []),
    stats := case when t.value_count > 0 then struct_pack(count := t.value_count,
        min := t.value_min, max := t.value_max, sum := t.value_sum, avg := t.value_sum / t.value_count) end,
    datapointCount := swc.datapoint_count, lastSeenNs := swc.latest_ns,
    rateStats := {{if .NoScalarPools}}null{{else}}srs.rate_stats{{end}},
    views := {{if .NoScalarPools}}null{{else}}sv.views{{end}},
    sparkline := {{if .NoScalarPools}}null{{else}}sp.sparkline{{end}}){{end}}
{{define "aggregateBucket"}}struct_pack(timestamp := f.timestamp, startTime := f.start_time,
    count := f.count, sum := f.sum, min := f.extents.min, max := f.extents.max,
    scale := f.scale, zeroThreshold := f.zero_threshold,
    zeroCount := f.zero_count + f.pos_fold.folded + f.neg_fold.folded,
    positiveBucketOffset := f.pos_fold.offset, positiveBucketCounts := f.pos_fold.counts,
    negativeBucketOffset := f.neg_fold.offset, negativeBucketCounts := f.neg_fold.counts,
    explicitBounds := f.explicit_bounds, bucketCounts := f.bucket_counts, quantiles := aq.quantiles){{end}}
{{define "contributionBucket"}}struct_pack(timestamp := p.bucket_start, startTime := p.start_time,
    count := p.count, sum := p.sum, min := p.extents.min, max := p.extents.max,
    scale := p.aggregate_scale, zeroThreshold := p.aggregate_zero_threshold,
    zeroCount := p.zero_count + p.pos_fold.folded + p.neg_fold.folded + p.final_pos.folded + p.final_neg.folded,
    positiveBucketOffset := p.final_pos.offset, positiveBucketCounts := p.final_pos.counts,
    negativeBucketOffset := p.final_neg.offset, negativeBucketCounts := p.final_neg.counts,
    explicitBounds := p.explicit_bounds, bucketCounts := p.bucket_counts, quantiles := null){{end}}
{{define "metricView"}}select
{{if not .AggregateOnly}}
    s.id::varchar as metricRef, s.name, s.description, s.unit, s.metadata as metadata,
    s.metric_type as metricType,
    case when s.metric_type in ('Sum', 'Histogram', 'ExponentialHistogram') then s.aggregation_temporality end as aggregationTemporalityCode,
    case when s.metric_type = 'Sum' then s.is_monotonic end as isMonotonic,
    s.resource_json as resource, s.scope_json as scope,
    coalesce((select timeseries from timeseries_agg), []) as timeseries,
    (select max_ts from data_extent) as lastSeenNs,
    coalesce((select sum(dp_count) from (select count(*) as dp_count from filtered_dps group by series_id)), 0) as datapointCount,
    {{if .NoHistogramMerge}}null{{else}}(select case when series_buckets > 0 or aggregate_buckets > 0
        then struct_pack(seriesBuckets := series_buckets, aggregateBuckets := aggregate_buckets) end from bounds_mismatch){{end}} as boundsMismatch,
    struct_pack(requested := struct_pack(startNs := list_extract({{bind "time_start"}}::ubigint[], 1), endNs := list_extract({{bind "time_end"}}::ubigint[], 1)),
        effective := struct_pack(startNs := (select start_ns from effective_window), endNs := (select end_ns from effective_window))) as "window",
{{end}}
    {{if .NoHistogramMerge}}null{{else}}(select aggregate from aggregate_agg){{end}} as aggregate,
{{if .ContributionID}}
    {{if .NoHistogramMerge}}[]{{else}}coalesce((select contribution from contribution_agg), []){{end}} as contribution,
{{end}}
    {{if .NoScalarPools}}struct_pack(selected := [], "all" := []){{else}}
    struct_pack(selected := coalesce((select selected from scalar_pools_json), []), "all" := coalesce((select all_series from scalar_pools_json), [])){{end}} as scalarAggregate
from {{if .AggregateOnly}}selected_metric{{else}}selected_metric_json{{end}} s
{{end}}
