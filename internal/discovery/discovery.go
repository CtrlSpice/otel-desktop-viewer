// Package discovery builds the read-only discovery queries shared by the CLI and UI.
package discovery

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type Query struct {
	StartTime *string
	EndTime   *string
	Service   string
	// ServiceName distinguishes an exact empty service name from no filter.
	ServiceName     *string
	Namespace       *string
	ServiceIdentity *string
	Sources         Sources
}

// Sources names server-owned relations that have already applied a signal query.
// Empty names use the stored signal tables.
type Sources struct {
	Spans   string
	Logs    string
	Metrics string
}

func source(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

type Location struct {
	Signal    string `json:"signal"`
	OwnerType string `json:"ownerType"`
}

func Literal(value string) string {
	// SQL source cannot contain NUL; decode the UTF-8 bytes inside DuckDB instead.
	if strings.ContainsRune(value, 0) {
		return "decode(from_hex('" + hex.EncodeToString([]byte(value)) + "'))"
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// ServiceIdentity retains received value kinds and exact wire values. OTel treats
// an absent namespace as an empty string; an absent name remains distinct.
// resource is a server-owned SQL alias, never query input.
func ServiceIdentity(resource string) string {
	value := func(key string) string {
		return "(SELECT a.value FROM attributes a WHERE a.key = " + Literal(key) + " AND list_contains(" + resource + ".attribute_ids, a.id))"
	}
	return "json_array(" + value("service.name") + ", coalesce(" + value("service.namespace") + ", '{\"kind\":\"string\",\"value\":\"\"}'::JSON))::VARCHAR"
}

func Predicate(query Query) string {
	p := []string{"true"}
	if query.StartTime != nil {
		p = append(p, "s.time_ns >= "+Literal(*query.StartTime)+"::UBIGINT")
	}
	if query.EndTime != nil {
		p = append(p, "s.time_ns <= "+Literal(*query.EndTime)+"::UBIGINT")
	}
	if query.ServiceName != nil {
		p = append(p, "s.service_name = "+Literal(*query.ServiceName))
	} else if query.Service != "" {
		p = append(p, "s.service_name = "+Literal(query.Service))
	}
	if query.Namespace != nil {
		p = append(p, `coalesce((SELECT attribute_text(a.value) FROM resources r, attributes a WHERE r.id = s.resource_id AND a.key = 'service.namespace' AND list_contains(r.attribute_ids, a.id)), '') = `+Literal(*query.Namespace))
	}
	if query.ServiceIdentity != nil {
		p = append(p, "(SELECT "+ServiceIdentity("r")+" FROM resources r WHERE r.id = s.resource_id) = "+Literal(*query.ServiceIdentity))
	}
	return strings.Join(p, " AND ")
}

// Records keeps counted record identity distinct from the attribute owner.
func Records(signal, ownerType string) (string, error) {
	return RecordsFrom(signal, ownerType, Sources{})
}

func RecordsFrom(signal, ownerType string, sources Sources) (string, error) {
	var identity, timestamp, service, from, directOwner, attributes, resource, scope, allowed string
	switch signal {
	case "traces":
		identity, timestamp, service = "struct_pack(trace_id := s.trace_id, span_id := s.span_id)", "s.start_time", "s.service_name"
		from, directOwner, attributes, resource, scope = source(sources.Spans, "spans")+" s", "span", "s.attribute_ids", "s.resource_id", "s.scope_id"
		allowed = "span, event, link, resource or scope"
		switch ownerType {
		case "event":
			from += " JOIN events e ON e.trace_id = s.trace_id AND e.span_id = s.span_id"
			attributes, directOwner, timestamp = "e.attribute_ids", "event", "e.timestamp"
		case "link":
			from += " JOIN links l ON l.trace_id = s.trace_id AND l.span_id = s.span_id"
			attributes, directOwner = "l.attribute_ids", "link"
		}
	case "logs":
		identity, timestamp, service = "l.id", "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)", "l.service_name"
		from, directOwner, attributes, resource, scope = source(sources.Logs, "logs")+" l", "log", "l.attribute_ids", "l.resource_id", "l.scope_id"
		allowed = "log, resource or scope"
	case "metrics":
		identity, timestamp, service = "d.id", "d.timestamp", "m.service_name"
		from, directOwner = "metric_datapoints d JOIN "+source(sources.Metrics, "metrics")+" m ON m.id = d.metric_id", "datapoint"
		attributes, resource, scope = "d.attribute_ids", "m.resource_id", "m.scope_id"
		allowed = "datapoint, exemplar, metadata, resource or scope"
		switch ownerType {
		case "exemplar":
			from += " JOIN exemplars e ON e.metric_datapoint_id = d.id"
			attributes, directOwner, timestamp = "e.attribute_ids", "exemplar", "e.timestamp"
		case "metadata":
			attributes, directOwner = "m.metadata_ids", "metadata"
		}
	default:
		return "", fmt.Errorf("unsupported --signal %q: use traces, logs or metrics", signal)
	}
	switch ownerType {
	case directOwner:
	case "resource":
		from += " JOIN resources r ON r.id = " + resource
		attributes = "r.attribute_ids"
	case "scope":
		from += " JOIN scopes sc ON sc.id = " + scope
		attributes = "sc.attribute_ids"
	default:
		return "", fmt.Errorf("--signal %s supports --owner-type %s; got %q", signal, allowed, ownerType)
	}
	return "SELECT " + identity + " AS record_id, " + timestamp + " AS time_ns, " + service + " AS service_name, " + resource + " AS resource_id, " + attributes + " AS attribute_ids FROM " + from, nil
}

func foundOn(location Location) string {
	return "json_array(json_object('signal', " + Literal(location.Signal) + ", 'ownerType', " + Literal(location.OwnerType) + "))"
}

func Keys(query Query, location Location, records string) string {
	return `WITH records AS (` + records + `)
SELECT json_object('key', a.key, 'kind', json_extract_string(a.value, '$.kind'),
    'foundOn', ` + foundOn(location) + `) AS entry
FROM records s
CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
JOIN attributes a ON a.id = owned.attribute_id
WHERE ` + Predicate(query) + `
GROUP BY a.key, json_extract_string(a.value, '$.kind')
ORDER BY a.key, json_extract_string(a.value, '$.kind')`
}

func Values(query Query, location Location, records, key string) string {
	return `WITH records AS (` + records + `), owned_values AS (
    SELECT s.record_id, a.value FROM records s
    CROSS JOIN unnest(s.attribute_ids) owned(attribute_id)
    JOIN attributes a ON a.id = owned.attribute_id
    WHERE ` + Predicate(query) + ` AND a.key = ` + Literal(key) + `
), value_counts AS (
    SELECT value, count(DISTINCT record_id) AS count FROM owned_values GROUP BY value
), kind_counts AS (
    SELECT json_extract_string(value, '$.kind') AS kind, count(DISTINCT record_id) AS count
    FROM owned_values GROUP BY json_extract_string(value, '$.kind')
), denominator AS (
    SELECT count(DISTINCT record_id) AS count FROM owned_values
)
SELECT json_object('value', v.value, 'foundOn', ` + foundOn(location) + `,
    'count', v.count, 'kindCount', k.count, 'denominator', d.count,
    'relativeFrequency', v.count::DOUBLE / nullif(d.count, 0)::DOUBLE) AS entry
FROM value_counts v JOIN kind_counts k ON k.kind = json_extract_string(v.value, '$.kind')
CROSS JOIN denominator d
ORDER BY v.count DESC, json_extract_string(v.value, '$.kind'), v.value::VARCHAR`
}

func Services(query Query) string {
	predicate := Predicate(query)
	return `WITH identities AS (
    SELECT id, service_identity,
        coalesce(attribute_text(json_extract(service_identity, '$[0]')), '') AS service_name,
        coalesce(attribute_text(json_extract(service_identity, '$[1]')), '') AS service_namespace
    FROM (SELECT r.id, ` + ServiceIdentity("r") + ` AS service_identity FROM resources r)
), exemplar_counts AS (
    SELECT metric_datapoint_id, count(*) AS exemplar_count FROM exemplars GROUP BY metric_datapoint_id
), service_counts AS (
    SELECT i.service_identity, i.service_namespace, i.service_name, count(DISTINCT trace_id) AS trace_count, count(*) AS span_count,
        count(*) FILTER (WHERE status_code = 2) AS error_span_count,
        max(time_ns) AS last_seen
    FROM (SELECT resource_id, service_name, trace_id, start_time AS time_ns, status_code FROM ` + source(query.Sources.Spans, "spans") + `) s
    JOIN identities i ON i.id = s.resource_id
    WHERE ` + predicate + ` GROUP BY i.service_identity, i.service_namespace, i.service_name
    UNION ALL BY NAME
    SELECT i.service_identity, i.service_namespace, i.service_name, count(*) AS log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 17 AND 24) AS error_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 1 AND 4) AS trace_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 5 AND 8) AS debug_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 9 AND 12) AS info_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 13 AND 16) AS warn_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 17 AND 20) AS error_severity_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 21 AND 24) AS fatal_log_count,
        count(*) FILTER (WHERE severity_number IS NULL OR severity_number NOT BETWEEN 1 AND 24) AS other_log_count,
        max(time_ns) AS last_seen
    FROM (SELECT resource_id, service_name,
        coalesce(nullif(timestamp, 0), observed_timestamp) AS time_ns, severity_number FROM ` + source(query.Sources.Logs, "logs") + `) s
    JOIN identities i ON i.id = s.resource_id
    WHERE ` + predicate + ` GROUP BY i.service_identity, i.service_namespace, i.service_name
    UNION ALL BY NAME
    SELECT i.service_identity, i.service_namespace, i.service_name, count(DISTINCT metric_id) AS metric_count,
        count(*) AS data_point_count, sum(coalesce(e.exemplar_count, 0)) AS exemplar_count, max(time_ns) AS last_seen
    FROM (SELECT m.resource_id, m.service_name, m.id AS metric_id, d.id AS datapoint_id, d.timestamp AS time_ns
        FROM metric_datapoints d JOIN ` + source(query.Sources.Metrics, "metrics") + ` m ON m.id = d.metric_id) s
    JOIN identities i ON i.id = s.resource_id
    LEFT JOIN exemplar_counts e ON e.metric_datapoint_id = s.datapoint_id
    WHERE ` + predicate + ` GROUP BY i.service_identity, i.service_namespace, i.service_name
)
SELECT json_object(
    'serviceNamespace', c.service_namespace, 'serviceName', c.service_name,
    'serviceIdentity', c.service_identity,
    'serviceNameKind', coalesce(json_extract_string(c.service_identity, '$[0].kind'), 'missing'),
    'serviceNamespaceKind', json_extract_string(c.service_identity, '$[1].kind'),
    'traceCount', coalesce(sum(c.trace_count), 0),
    'spanCount', coalesce(sum(c.span_count), 0), 'errorSpanCount', coalesce(sum(c.error_span_count), 0),
    'logCount', coalesce(sum(c.log_count), 0), 'errorLogCount', coalesce(sum(c.error_log_count), 0),
    'logSeverityCounts', json_object(
        'trace', coalesce(sum(c.trace_log_count), 0), 'debug', coalesce(sum(c.debug_log_count), 0),
        'info', coalesce(sum(c.info_log_count), 0), 'warn', coalesce(sum(c.warn_log_count), 0),
        'error', coalesce(sum(c.error_severity_log_count), 0), 'fatal', coalesce(sum(c.fatal_log_count), 0),
        'other', coalesce(sum(c.other_log_count), 0)),
    'metricCount', coalesce(sum(c.metric_count), 0), 'dataPointCount', coalesce(sum(c.data_point_count), 0),
    'exemplarCount', coalesce(sum(c.exemplar_count), 0),
    'lastSeen', max(c.last_seen)::VARCHAR) AS entry
FROM service_counts c
GROUP BY c.service_identity, c.service_namespace, c.service_name
ORDER BY c.service_namespace, c.service_name, c.service_identity`
}
