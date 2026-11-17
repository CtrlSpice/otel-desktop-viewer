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
		p = append(p, "s.time_ns >= "+b.Bind(*query.StartTime)+"::ubigint")
	}
	if query.EndTime != nil {
		p = append(p, "s.time_ns <= "+b.Bind(*query.EndTime)+"::ubigint")
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
	return strings.Join(p, " and ")
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
			p = append(p, field+" >= "+b.Bind(*query.StartTime)+"::ubigint")
		}
		if query.EndTime != nil {
			p = append(p, field+" <= "+b.Bind(*query.EndTime)+"::ubigint")
		}
		return strings.Join(p, " and ")
	}
	var identity, timestamp, service, from, directOwner, attributes, resource, scope, allowed string
	switch signal {
	case "traces":
		identity, timestamp, service = "struct_pack(trace_id := s.trace_id, span_id := s.span_id)", "s.start_time", "s.service_name"
		from, directOwner, attributes, resource, scope = source(sources.Spans, "spans")+" s", "span", "s.attributes", "s.resource", "s.scope"
		allowed = "span, event, link, resource or scope"
		switch ownerType {
		case "event":
			from += " cross join unnest(list_filter(s.events, lambda e: " + window("e.timestamp") + ")) as child(e)"
			attributes, directOwner, timestamp = "e.attributes", "event", "e.timestamp"
		case "link":
			from += " cross join unnest(s.links) as child(l)"
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
			selection = "exists (select 1 from " + selected + " m where m.id = r.id)"
		}
		identity, timestamp, service = "r.d.id", "r.d.timestamp", "r.service_name"
		points, overlap := "r.datapoints", "true"
		if ownerType != "exemplar" {
			points = "list_filter(r.datapoints, lambda d: " + window("d.timestamp") + ")"
			if query.StartTime != nil {
				overlap += " and r.point_time_max >= " + b.Bind(*query.StartTime) + "::ubigint"
			}
			if query.EndTime != nil {
				overlap += " and r.point_time_min <= " + b.Bind(*query.EndTime) + "::ubigint"
			}
		}
		from, directOwner = "(select r.*, unnest("+points+") as d from metric_records r where "+overlap+" and "+selection+") r", "datapoint"
		attributes, resource, scope = "r.d.attributes", "r.resource", "r.scope"
		allowed = "datapoint, exemplar, metadata, resource or scope"
		switch ownerType {
		case "exemplar":
			from += " cross join unnest(list_filter(r.d.exemplars, lambda e: " + window("e.timestamp") + ")) as child(e)"
			attributes, directOwner, timestamp = "e.attributes", "exemplar", "e.timestamp"
		case "metadata":
			// Existing discovery exposes the latest logical descriptor's metadata,
			// not received-record metadata. Preserve that contract explicitly.
			attributes, directOwner = "(select m.metadata from "+selected+" m where m.id = r.id)", "metadata"
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
	return "select " + identity + " as record_id, " + timestamp + " as time_ns, " + service + " as service_name, " + resource + " as resource, " + attributes + " as attributes from " + from, nil
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
	return `with scoped_records as (` + records + `), contexts as (
    select distinct variant_to_parquet_variant(s.attributes)::struct(metadata blob, value blob) as parts
    from scoped_records s where ` + b.Predicate(query) + `
), keys as (
    select a.key, otel_value_kind(attribute_native_field(c.parts, a.key)) as kind
    from contexts c, unnest(variant_keys(variant_bytes_to_variant(c.parts.metadata || c.parts.value))) a(key)
    group by a.key, kind
)
select ` + object(query, "key", "key", "kind", "kind", "foundOn", b.foundOn(query, location)) + ` as entry
from keys order by key, kind`
}

func (b *Builder) Values(query Query, location Location, records, key string) string {
	value := "v.identity::json"
	if query.Native {
		value = "v.value"
	}
	fields := []string{"value", value, "foundOn", b.foundOn(query, location),
		"count", "v.count", "kindCount", "k.count", "denominator", "d.count",
		"relativeFrequency", "v.count::double / nullif(d.count,0)::double"}
	if query.Native {
		fields = append(fields, "identity", "v.identity")
	}
	// Encode canonical identities once per distinct received value, not once per
	// owner. Both transports retain the same grouping and record denominators.
	// Native output also preserves a deterministic representative's received bits.
	return `with records as (` + records + `), received_values as materialized (
    select s.record_id,
        variant_to_parquet_variant(s.attributes[` + b.Bind(key) + `])::struct(metadata blob, value blob) as parts
    from records s where ` + b.Predicate(query) + ` and variant_exists(s.attributes, ` + b.Bind(key) + `)
), distinct_values as materialized (
    select distinct parts from received_values
), identified_values as materialized (
    select parts, variant_bytes_to_variant(parts.metadata || parts.value) as value,
        otel_value_json(value)::varchar as identity from distinct_values
), owned_values as (
    select r.record_id, i.parts, i.value, i.identity
    from received_values r join identified_values i on r.parts is not distinct from i.parts
), value_counts as (
    select identity, first(value order by record_id, parts) as value,
        count(distinct record_id) as count from owned_values group by identity
), kind_counts as (
    select otel_value_kind(value) as kind, count(distinct record_id) as count from owned_values group by otel_value_kind(value)
), denominator as (select count(distinct record_id) as count from owned_values)
select ` + object(query, fields...) + ` as entry
from value_counts v join kind_counts k on k.kind=otel_value_kind(v.value) cross join denominator d
order by v.count desc, otel_value_kind(v.value), v.identity`
}

func (b *Builder) Services(query Query) string {
	// Time/search eligibility precedes grouping. Compact resource IDs join the
	// ingestion-time service catalogue; reads do not decode resource attributes.
	predicate := b.Predicate(Query{StartTime: query.StartTime, EndTime: query.EndTime})
	resourceParts := `s.resource.id as resource_id`
	metricSelection := "true"
	if query.Sources.Metrics != "" {
		metricSelection = "exists (select 1 from " + query.Sources.Metrics + " m where m.id=r.id)"
	}
	lastSeen := "max(c.last_seen)::varchar"
	if query.Native {
		lastSeen = "max(c.last_seen)"
	}
	name, namespace := "c.service_name", "c.service_namespace"
	if query.Native {
		name, namespace = "c.name_preview", "c.namespace_preview"
	}
	entry := object(query,
		"serviceColorKey", "case when octet_length(encode(c.service_name))>300 then 'sha256:'||sha256(c.service_name) when starts_with(c.service_name,'sha256:') or starts_with(c.service_name,'name:') then 'name:'||c.service_name else c.service_name end",
		"labelsComplete", "c.service_name=c.name_preview and c.service_namespace=c.namespace_preview",
		"serviceNamespace", namespace, "serviceName", name, "serviceIdentity", "c.service_identity",
		"serviceNameKind", "c.name_kind",
		"serviceNamespaceKind", "c.namespace_kind",
		"traceCount", "len(list_distinct(flatten(list(coalesce(c.trace_ids, []::uuid[])))))", "spanCount", "coalesce(sum(c.span_count),0)", "errorSpanCount", "coalesce(sum(c.error_span_count),0)",
		"logCount", "coalesce(sum(c.log_count),0)", "errorLogCount", "coalesce(sum(c.error_log_count),0)",
		"logSeverityCounts", object(query, "trace", "coalesce(sum(c.trace_log_count),0)", "debug", "coalesce(sum(c.debug_log_count),0)",
			"info", "coalesce(sum(c.info_log_count),0)", "warn", "coalesce(sum(c.warn_log_count),0)", "error", "coalesce(sum(c.error_severity_log_count),0)",
			"fatal", "coalesce(sum(c.fatal_log_count),0)", "other", "coalesce(sum(c.other_log_count),0)"),
		"metricCount", "len(list_distinct(flatten(list(coalesce(c.metric_ids, []::uuid[])))))", "dataPointCount", "coalesce(sum(c.data_point_count),0)", "exemplarCount", "coalesce(sum(c.exemplar_count),0)", "lastSeen", lastSeen)
	return `with resource_counts as materialized (
    select ` + resourceParts + `, list(distinct trace_id) as trace_ids, count(*) as span_count,
        count(*) filter (where status_code = 2) as error_span_count,
        max(time_ns) as last_seen
    from (select resource, service_name, trace_id, start_time as time_ns, status_code from ` + source(query.Sources.Spans, "spans") + `) s
    where ` + predicate + ` group by all
    union all by name
    select ` + resourceParts + `, count(*) as log_count,
        count(*) filter (where severity_number between 17 and 24) as error_log_count,
        count(*) filter (where severity_number between 1 and 4) as trace_log_count,
        count(*) filter (where severity_number between 5 and 8) as debug_log_count,
        count(*) filter (where severity_number between 9 and 12) as info_log_count,
        count(*) filter (where severity_number between 13 and 16) as warn_log_count,
        count(*) filter (where severity_number between 17 and 20) as error_severity_log_count,
        count(*) filter (where severity_number between 21 and 24) as fatal_log_count,
        count(*) filter (where severity_number is null or severity_number not between 1 and 24) as other_log_count,
        max(time_ns) as last_seen
    from (select resource, service_name,
        coalesce(nullif(timestamp, 0), observed_timestamp) as time_ns, severity_number from ` + source(query.Sources.Logs, "logs") + `) s
    where ` + predicate + ` group by all
    union all by name
    select ` + resourceParts + `, list(distinct metric_id) as metric_ids,
        count(*) as data_point_count, sum(len(exemplars)) as exemplar_count, max(time_ns) as last_seen
    from (select r.resource, r.service_name, r.id as metric_id, r.d.timestamp as time_ns, r.d.exemplars as exemplars
        from (select r.*, unnest(r.datapoints) as d from metric_records r where ` + metricSelection + `) r) s
    where ` + predicate + ` group by all
), service_counts as (
    select c.* exclude(resource_id), s.ref as service_identity, s.name as service_name, s.namespace as service_namespace,
        s.name_kind, s.namespace_kind, s.name_preview, s.namespace_preview
    from resource_counts c join search_service_resources r using(resource_id) join search_services s on s.ref=r.service_ref
    where ` + b.serviceLabelPredicate(query) + `
)
select ` + entry + ` as entry
from service_counts c
group by c.service_identity, c.service_namespace, c.service_name, c.name_kind, c.namespace_kind, c.name_preview, c.namespace_preview
order by c.service_namespace, c.service_name, c.service_identity`
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
	return strings.Join(p, " and ")
}

// ServiceCount is database-wide, like navigation's signal counts. It does not
// construct service summaries, sort/page them, or expand metric datapoints.
func ServiceCount() string {
	return `with resources as (
    select resource.id as resource_id from spans
    union
    select resource.id from logs
    union
    select resource.id from metric_records where len(datapoints)>0
) select count(distinct service_ref) from resources join search_service_resources using(resource_id)`
}
