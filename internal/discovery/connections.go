package discovery

// connectionRecords preserves actual parent/child references. Both endpoints
// must be present in the selected trace source; names and timestamps never
// establish a connection. Service identity includes the namespace and types.
func connectionRecords(query Query) string {
	return `WITH selected_spans AS (
    SELECT s.*, ` + ServiceIdentity("s.resource") + ` AS identity
    FROM (SELECT *, start_time AS time_ns FROM ` + source(query.Sources.Spans, "spans") + `) s
    WHERE ` + Predicate(Query{StartTime: query.StartTime, EndTime: query.EndTime}) + `
), connections AS (
    SELECT p.identity AS source, c.identity AS target, c.trace_id, c.span_id,
        c.name, c.start_time, c.end_time, c.status_code
    FROM selected_spans c JOIN selected_spans p
      ON p.trace_id = c.trace_id AND p.span_id = c.parent_span_id
    WHERE p.identity <> c.identity
) `
}

// ServiceConnections returns directed, aggregated neighbours of one service.
func ServiceConnections(query Query) string {
	identity := Literal(*query.ServiceIdentity)
	unscoped := query
	unscoped.ServiceIdentity, unscoped.ServiceName, unscoped.Namespace = nil, nil, nil
	return connectionRecords(query) + `, summaries AS (` + Services(unscoped) + `), grouped AS (
    SELECT source, target, count(*) AS span_count, count(DISTINCT trace_id) AS trace_count,
        count(*) FILTER (WHERE status_code = 2) AS error_count
    FROM connections WHERE source = ` + identity + ` OR target = ` + identity + `
    GROUP BY source, target
)
SELECT json_object('source', s.entry, 'target', t.entry,
    'spanCount', g.span_count, 'traceCount', g.trace_count, 'errorSpanCount', g.error_count) AS entry
FROM grouped g
JOIN summaries s ON json_extract_string(s.entry, '$.serviceIdentity') = g.source
JOIN summaries t ON json_extract_string(t.entry, '$.serviceIdentity') = g.target`
}

// ServiceConnectionSpans returns the received child spans behind one edge.
func ServiceConnectionSpans(query Query, targetIdentity string) string {
	return connectionRecords(query) + `SELECT json_object(
    'traceID', replace(trace_id::VARCHAR, '-', ''),
    'spanID', lower(lpad(hex(span_id), 16, '0')), 'name', name,
    'startTime', start_time::VARCHAR, 'endTime', end_time::VARCHAR,
    'statusCode', status_code) AS entry
FROM connections WHERE source = ` + Literal(*query.ServiceIdentity) + ` AND target = ` + Literal(targetIdentity)
}
