package discovery

// connectionRecords preserves actual parent/child references. Both endpoints
// must be present in the selected trace source; names and timestamps never
// establish a connection. Service identity includes the namespace and types.
func (b *Builder) connectionRecords(query Query) string {
	return `with selected_spans as (
    select s.*, ` + ServiceIdentity("s.resource") + ` as identity
    from (select *, start_time as time_ns from ` + source(query.Sources.Spans, "spans") + `) s
    where ` + b.Predicate(Query{StartTime: query.StartTime, EndTime: query.EndTime}) + `
), connections as (
    select p.identity as source, c.identity as target, c.trace_id, c.span_id,
        c.name, c.start_time, c.end_time, c.status_code
    from selected_spans c join selected_spans p
      on p.trace_id = c.trace_id and p.span_id = c.parent_span_id
    where p.identity <> c.identity
) `
}

// ServiceConnections returns directed, aggregated neighbours of one service.
func (b *Builder) ServiceConnections(query Query) string {
	identity := b.Bind(*query.ServiceIdentity)
	unscoped := query
	unscoped.ServiceIdentity, unscoped.ServiceName, unscoped.Namespace = nil, nil, nil
	entry := object(query, "source", "s.entry", "target", "t.entry", "spanCount", "g.span_count", "traceCount", "g.trace_count", "errorSpanCount", "g.error_count")
	sourceID, targetID := "json_extract_string(s.entry, '$.serviceIdentity')", "json_extract_string(t.entry, '$.serviceIdentity')"
	if query.Native {
		sourceID, targetID = "s.entry.serviceIdentity", "t.entry.serviceIdentity"
	}
	return b.connectionRecords(query) + `, summaries as (` + b.Services(unscoped) + `), grouped as (
    select source, target, count(*) as span_count, count(distinct trace_id) as trace_count,
        count(*) filter (where status_code = 2) as error_count
    from connections where source = ` + identity + ` or target = ` + identity + `
    group by source, target
)
select ` + entry + ` as entry
from grouped g
join summaries s on ` + sourceID + ` = g.source
join summaries t on ` + targetID + ` = g.target`
}

// ServiceConnectionSpans returns the received child spans behind one edge.
func (b *Builder) ServiceConnectionSpans(query Query, targetIdentity string) string {
	start, end := "start_time::varchar", "end_time::varchar"
	if query.Native {
		start, end = "start_time", "end_time"
	}
	entry := object(query, "traceID", "replace(trace_id::varchar, '-', '')", "spanID", "lower(lpad(hex(span_id),16,'0'))", "name", "name", "startTime", start, "endTime", end, "statusCode", "status_code")
	return b.connectionRecords(query) + `select ` + entry + ` as entry
from connections where source = ` + b.Bind(*query.ServiceIdentity) + ` and target = ` + b.Bind(targetIdentity)
}
