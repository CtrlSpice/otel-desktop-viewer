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
	// Native emits nested SQL values for Arrow datasets instead of JSON documents.
	Native bool
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

// ServiceIdentity is the catalogue's compact typed name/namespace reference.
// resource is a server-owned SQL alias, never query input.
func ServiceIdentity(resource string) string {
	return "(select sr.service_ref from search_service_resources sr where sr.resource_id=" + resource + ".id)"
}

func (b *Builder) Predicate(query Query) string {
	p := []string{"true"}
	if query.StartTime != nil {
		p = append(p, "s.time_ns >= "+b.Bind(*query.StartTime)+"::UBIGINT")
	}
	if query.EndTime != nil {
		p = append(p, "s.time_ns <= "+b.Bind(*query.EndTime)+"::UBIGINT")
	}
	if query.ServiceName != nil {
		p = append(p, "s.service_name = "+b.Bind(*query.ServiceName))
	} else if query.Service != "" {
		p = append(p, "s.service_name = "+b.Bind(query.Service))
	}
	if query.Namespace != nil {
		p = append(p, `coalesce(otel_value_text(s.resource.attributes['service.namespace']), '') = `+b.Bind(*query.Namespace))
	}
	if query.ServiceIdentity != nil {
		p = append(p, ServiceIdentity("s.resource")+" = "+b.Bind(*query.ServiceIdentity))
	}
	return strings.Join(p, " AND ")
}

// Records keeps counted record identity distinct from the attribute owner.
func (b *Builder) Records(signal, ownerType string) (string, error) {
	return b.RecordsFrom(signal, ownerType, Query{})
}

func (b *Builder) RecordsFrom(signal, ownerType string, query Query) (string, error) {
	sources := query.Sources
	window := func(field string) string {
		p := []string{"true"}
		if query.StartTime != nil {
			p = append(p, field+" >= "+b.Bind(*query.StartTime)+"::UBIGINT")
		}
		if query.EndTime != nil {
			p = append(p, field+" <= "+b.Bind(*query.EndTime)+"::UBIGINT")
		}
		return strings.Join(p, " AND ")
	}
	var identity, timestamp, service, from, directOwner, attributes, resource, scope, allowed string
	switch signal {
	case "traces":
		identity, timestamp, service = "struct_pack(trace_id := s.trace_id, span_id := s.span_id)", "s.start_time", "s.service_name"
		from, directOwner, attributes, resource, scope = source(sources.Spans, "spans")+" s", "span", "s.attributes", "s.resource", "s.scope"
		allowed = "span, event, link, resource or scope"
		switch ownerType {
		case "event":
			from += " CROSS JOIN unnest(list_filter(s.events, lambda e: " + window("e.timestamp") + ")) AS child(e)"
			attributes, directOwner, timestamp = "e.attributes", "event", "e.timestamp"
		case "link":
			from += " CROSS JOIN unnest(s.links) AS child(l)"
			attributes, directOwner = "l.attributes", "link"
		}
	case "logs":
		identity, timestamp, service = "l.id", "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)", "l.service_name"
		from, directOwner, attributes, resource, scope = source(sources.Logs, "logs")+" l", "log", "l.attributes", "l.resource", "l.scope"
		allowed = "log, resource or scope"
	case "metrics":
		// Select logical metrics before expanding their received owning records.
		// The selected relation owns whole-expression search semantics; do not
		// rewrite its predicate against individual points or received descriptors.
		selected := source(sources.Metrics, "metrics")
		selection := "true"
		if sources.Metrics != "" {
			selection = "EXISTS (SELECT 1 FROM " + selected + " m WHERE m.id = r.id)"
		}
		identity, timestamp, service = "r.d.id", "r.d.timestamp", "r.service_name"
		points, overlap := "r.datapoints", "true"
		if ownerType != "exemplar" {
			points = "list_filter(r.datapoints, lambda d: " + window("d.timestamp") + ")"
			if query.StartTime != nil {
				overlap += " AND r.point_time_max >= " + b.Bind(*query.StartTime) + "::UBIGINT"
			}
			if query.EndTime != nil {
				overlap += " AND r.point_time_min <= " + b.Bind(*query.EndTime) + "::UBIGINT"
			}
		}
		from, directOwner = "(SELECT r.*, unnest("+points+") AS d FROM metric_records r WHERE "+overlap+" AND "+selection+") r", "datapoint"
		attributes, resource, scope = "r.d.attributes", "r.resource", "r.scope"
		allowed = "datapoint, exemplar, metadata, resource or scope"
		switch ownerType {
		case "exemplar":
			from += " CROSS JOIN unnest(list_filter(r.d.exemplars, lambda e: " + window("e.timestamp") + ")) AS child(e)"
			attributes, directOwner, timestamp = "e.attributes", "exemplar", "e.timestamp"
		case "metadata":
			// Existing discovery exposes the latest logical descriptor's metadata,
			// not received-record metadata. Preserve that contract explicitly.
			attributes, directOwner = "(SELECT m.metadata FROM "+selected+" m WHERE m.id = r.id)", "metadata"
		}
	default:
		return "", fmt.Errorf("unsupported --signal %q: use traces, logs or metrics", signal)
	}
	switch ownerType {
	case directOwner:
	case "resource":
		attributes = resource + ".attributes"
	case "scope":
		attributes = scope + ".attributes"
	default:
		return "", fmt.Errorf("--signal %s supports --owner-type %s; got %q", signal, allowed, ownerType)
	}
	return "SELECT " + identity + " AS record_id, " + timestamp + " AS time_ns, " + service + " AS service_name, " + resource + " AS resource, " + attributes + " AS attributes FROM " + from, nil
}

func (b *Builder) foundOn(query Query, location Location) string {
	if query.Native {
		return "[struct_pack(signal := " + b.Bind(location.Signal) + ", ownerType := " + b.Bind(location.OwnerType) + ")]"
	}
	return "json_array(json_object('signal', " + b.Bind(location.Signal) + ", 'ownerType', " + b.Bind(location.OwnerType) + "))"
}

// object changes only the output representation; selection and aggregation are shared.
func object(query Query, fields ...string) string {
	parts := make([]string, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		if query.Native {
			parts = append(parts, `"`+fields[i]+`" := `+fields[i+1])
		} else {
			parts = append(parts, Literal(fields[i])+", "+fields[i+1])
		}
	}
	if query.Native {
		return "struct_pack(" + strings.Join(parts, ", ") + ")"
	}
	return "json_object(" + strings.Join(parts, ", ") + ")"
}

func (b *Builder) Keys(query Query, location Location, records string) string {
	// Keys need no record denominators. Enumerate each exact received attribute
	// context once, rather than encode the entire context as JSON per record.
	return `WITH scoped_records AS (` + records + `), contexts AS (
    SELECT DISTINCT variant_to_parquet_variant(s.attributes)::struct(metadata blob, value blob) AS parts
    FROM scoped_records s WHERE ` + b.Predicate(query) + `
), keys AS (
    SELECT a.key, otel_value_kind(attribute_native_field(c.parts, a.key)) AS kind
    FROM contexts c, unnest(variant_keys(variant_bytes_to_variant(c.parts.metadata || c.parts.value))) a(key)
    GROUP BY a.key, kind
)
SELECT ` + object(query, "key", "key", "kind", "kind", "foundOn", b.foundOn(query, location)) + ` AS entry
FROM keys ORDER BY key, kind`
}

func (b *Builder) Values(query Query, location Location, records, key string) string {
	value := "v.identity::JSON"
	if query.Native {
		value = "v.value"
	}
	fields := []string{"value", value, "foundOn", b.foundOn(query, location),
		"count", "v.count", "kindCount", "k.count", "denominator", "d.count",
		"relativeFrequency", "v.count::DOUBLE / nullif(d.count,0)::DOUBLE"}
	if query.Native {
		fields = append(fields, "identity", "v.identity")
	}
	// Encode canonical identities once per distinct received value, not once per
	// owner. Both transports retain the same grouping and record denominators.
	// Native output also preserves a deterministic representative's received bits.
	return `WITH records AS (` + records + `), received_values AS MATERIALIZED (
    SELECT s.record_id,
        variant_to_parquet_variant(s.attributes[` + b.Bind(key) + `])::struct(metadata blob, value blob) AS parts
    FROM records s WHERE ` + b.Predicate(query) + ` AND variant_exists(s.attributes, ` + b.Bind(key) + `)
), distinct_values AS MATERIALIZED (
    SELECT DISTINCT parts FROM received_values
), identified_values AS MATERIALIZED (
    SELECT parts, variant_bytes_to_variant(parts.metadata || parts.value) AS value,
        otel_value_json(value)::VARCHAR AS identity FROM distinct_values
), owned_values AS (
    SELECT r.record_id, i.parts, i.value, i.identity
    FROM received_values r JOIN identified_values i ON r.parts IS NOT DISTINCT FROM i.parts
), value_counts AS (
    SELECT identity, first(value ORDER BY record_id, parts) AS value,
        count(DISTINCT record_id) AS count FROM owned_values GROUP BY identity
), kind_counts AS (
    SELECT otel_value_kind(value) AS kind, count(DISTINCT record_id) AS count FROM owned_values GROUP BY otel_value_kind(value)
), denominator AS (SELECT count(DISTINCT record_id) AS count FROM owned_values)
SELECT ` + object(query, fields...) + ` AS entry
FROM value_counts v JOIN kind_counts k ON k.kind=otel_value_kind(v.value) CROSS JOIN denominator d
ORDER BY v.count DESC, otel_value_kind(v.value), v.identity`
}

func (b *Builder) Services(query Query) string {
	// Time/search eligibility precedes grouping. Compact resource IDs join the
	// ingestion-time service catalogue; reads do not decode resource attributes.
	predicate := b.Predicate(Query{StartTime: query.StartTime, EndTime: query.EndTime})
	resourceParts := `s.resource.id AS resource_id`
	metricSelection := "true"
	if query.Sources.Metrics != "" {
		metricSelection = "EXISTS (SELECT 1 FROM " + query.Sources.Metrics + " m WHERE m.id=r.id)"
	}
	lastSeen := "max(c.last_seen)::VARCHAR"
	if query.Native {
		lastSeen = "max(c.last_seen)"
	}
	name, namespace := "c.service_name", "c.service_namespace"
	if query.Native {
		name, namespace = "c.name_preview", "c.namespace_preview"
	}
	entry := object(query,
		"serviceColorKey", "case when octet_length(encode(c.service_name))>300 then 'sha256:'||sha256(c.service_name) when starts_with(c.service_name,'sha256:') or starts_with(c.service_name,'name:') then 'name:'||c.service_name else c.service_name end",
		"labelsComplete", "c.service_name=c.name_preview AND c.service_namespace=c.namespace_preview",
		"serviceNamespace", namespace, "serviceName", name, "serviceIdentity", "c.service_identity",
		"serviceNameKind", "c.name_kind",
		"serviceNamespaceKind", "c.namespace_kind",
		"traceCount", "len(list_distinct(flatten(list(coalesce(c.trace_ids, []::UUID[])))))", "spanCount", "coalesce(sum(c.span_count),0)", "errorSpanCount", "coalesce(sum(c.error_span_count),0)",
		"logCount", "coalesce(sum(c.log_count),0)", "errorLogCount", "coalesce(sum(c.error_log_count),0)",
		"logSeverityCounts", object(query, "trace", "coalesce(sum(c.trace_log_count),0)", "debug", "coalesce(sum(c.debug_log_count),0)",
			"info", "coalesce(sum(c.info_log_count),0)", "warn", "coalesce(sum(c.warn_log_count),0)", "error", "coalesce(sum(c.error_severity_log_count),0)",
			"fatal", "coalesce(sum(c.fatal_log_count),0)", "other", "coalesce(sum(c.other_log_count),0)"),
		"metricCount", "len(list_distinct(flatten(list(coalesce(c.metric_ids, []::UUID[])))))", "dataPointCount", "coalesce(sum(c.data_point_count),0)", "exemplarCount", "coalesce(sum(c.exemplar_count),0)", "lastSeen", lastSeen)
	return `WITH resource_counts AS MATERIALIZED (
    SELECT ` + resourceParts + `, list(DISTINCT trace_id) AS trace_ids, count(*) AS span_count,
        count(*) FILTER (WHERE status_code = 2) AS error_span_count,
        max(time_ns) AS last_seen
    FROM (SELECT resource, service_name, trace_id, start_time AS time_ns, status_code FROM ` + source(query.Sources.Spans, "spans") + `) s
    WHERE ` + predicate + ` GROUP BY ALL
    UNION ALL BY NAME
    SELECT ` + resourceParts + `, count(*) AS log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 17 AND 24) AS error_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 1 AND 4) AS trace_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 5 AND 8) AS debug_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 9 AND 12) AS info_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 13 AND 16) AS warn_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 17 AND 20) AS error_severity_log_count,
        count(*) FILTER (WHERE severity_number BETWEEN 21 AND 24) AS fatal_log_count,
        count(*) FILTER (WHERE severity_number IS NULL OR severity_number NOT BETWEEN 1 AND 24) AS other_log_count,
        max(time_ns) AS last_seen
    FROM (SELECT resource, service_name,
        coalesce(nullif(timestamp, 0), observed_timestamp) AS time_ns, severity_number FROM ` + source(query.Sources.Logs, "logs") + `) s
    WHERE ` + predicate + ` GROUP BY ALL
    UNION ALL BY NAME
    SELECT ` + resourceParts + `, list(DISTINCT metric_id) AS metric_ids,
        count(*) AS data_point_count, sum(len(exemplars)) AS exemplar_count, max(time_ns) AS last_seen
    FROM (SELECT r.resource, r.service_name, r.id AS metric_id, r.d.timestamp AS time_ns, r.d.exemplars AS exemplars
        FROM (SELECT r.*, unnest(r.datapoints) AS d FROM metric_records r WHERE ` + metricSelection + `) r) s
    WHERE ` + predicate + ` GROUP BY ALL
), service_counts AS (
    SELECT c.* EXCLUDE(resource_id), s.ref AS service_identity, s.name AS service_name, s.namespace AS service_namespace,
        s.name_kind, s.namespace_kind, s.name_preview, s.namespace_preview
    FROM resource_counts c JOIN search_service_resources r USING(resource_id) JOIN search_services s ON s.ref=r.service_ref
    WHERE ` + b.serviceLabelPredicate(query) + `
)
SELECT ` + entry + ` AS entry
FROM service_counts c
GROUP BY c.service_identity, c.service_namespace, c.service_name, c.name_kind, c.namespace_kind, c.name_preview, c.namespace_preview
ORDER BY c.service_namespace, c.service_name, c.service_identity`
}

func (b *Builder) serviceLabelPredicate(query Query) string {
	p := []string{"true"}
	if query.ServiceName != nil {
		p = append(p, "s.name = "+b.Bind(*query.ServiceName))
	} else if query.Service != "" {
		p = append(p, "s.name = "+b.Bind(query.Service))
	}
	if query.Namespace != nil {
		p = append(p, "s.namespace = "+b.Bind(*query.Namespace))
	}
	if query.ServiceIdentity != nil {
		p = append(p, "s.ref = "+b.Bind(*query.ServiceIdentity))
	}
	return strings.Join(p, " AND ")
}

// ServiceCount is database-wide, like navigation's signal counts. It does not
// construct service summaries, sort/page them, or expand metric datapoints.
func ServiceCount() string {
	return `WITH resources AS (
    SELECT resource.id AS resource_id FROM spans
    UNION
    SELECT resource.id FROM logs
    UNION
    SELECT resource.id FROM metric_records WHERE len(datapoints)>0
) SELECT count(DISTINCT service_ref) FROM resources JOIN search_service_resources USING(resource_id)`
}
