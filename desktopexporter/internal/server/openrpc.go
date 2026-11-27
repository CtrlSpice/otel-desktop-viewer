package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Released metadata, not the upstream source template (whose version is a placeholder).
const openRPCMetaSchema = "https://github.com/open-rpc/meta-schema/releases/download/1.14.9/open-rpc-meta-schema.json"

type rpcDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Schema      map[string]any `json:"schema"`
}

type rpcMethodDescription struct {
	Summary        string           `json:"summary"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	ParamStructure string           `json:"paramStructure"`
	Params         []rpcDescriptor  `json:"params"`
	Result         rpcDescriptor    `json:"result"`
	Errors         []map[string]any `json:"errors,omitempty"`
	Examples       []map[string]any `json:"examples,omitempty"`
}

func rpcSchemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func rpcObject(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func rpcArray(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func rpcRecord(fields map[string]any) map[string]any {
	return rpcObject(fields, slices.Sorted(maps.Keys(fields))...)
}

func rpcDiscoveryRows(service, location, text, count, timestamp map[string]any) map[string]any {
	number := map[string]any{"type": "number"}
	page := func(kind string, entry map[string]any) map[string]any {
		return map[string]any{"title": kind, "type": "array", "items": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": entry}}
	}
	services := page("services", service)
	rows := map[string]any{"anyOf": []any{
		services,
		page("connections", rpcRecord(map[string]any{"source": service, "target": service, "spanCount": count, "traceCount": count, "errorSpanCount": count})),
		page("connection-spans", rpcRecord(map[string]any{"traceID": rpcSchemaRef("EntityID"), "spanID": map[string]any{"type": "string", "pattern": "^[0-9a-f]{16}$"},
			"name": text, "startTime": timestamp, "endTime": timestamp, "statusCode": map[string]any{"type": "integer"}})),
		page("keys", rpcRecord(map[string]any{"key": text, "kind": text, "foundOn": rpcArray(location)})),
		page("values", rpcRecord(map[string]any{"value": rpcSchemaRef("Value"), "count": count, "kindCount": count, "denominator": count, "relativeFrequency": number, "foundOn": rpcArray(location)})),
	}}
	column := rpcRecord(map[string]any{"name": map[string]any{"const": "entry"}, "type": map[string]any{"const": "JSON"}})
	result := rpcObject(map[string]any{"rows": rows, "columns": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": column}, "truncated": map[string]any{"type": "boolean"},
		"totals": rpcRecord(map[string]any{"home": count, "traces": count, "logs": count, "metrics": count})}, "rows", "columns", "truncated")
	result["dependencies"] = map[string]any{"totals": map[string]any{"properties": map[string]any{"rows": services}}}
	return result
}

func openRPCDocument() map[string]any {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	object := map[string]any{"type": "object"}
	decimal := map[string]any{"type": "string", "pattern": "^-?[0-9]+$", "description": "Exact decimal integer text. Never decode through a floating-point number."}
	uintInput := map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "pattern": "^[0-9]+$"},
		map[string]any{"type": "integer", "minimum": 0, "maximum": json.Number("18446744073709551615")},
	}, "description": "Unsigned 64-bit integer. Prefer a decimal string to avoid floating-point rounding. The server rejects fractions and exponent notation."}
	intInput := map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "pattern": "^[+-]?[0-9]+$"},
		map[string]any{"type": "integer", "minimum": json.Number("-9223372036854775808"), "maximum": json.Number("9223372036854775807")},
	}, "description": "Signed 64-bit integer. Prefer a decimal string to avoid floating-point rounding. The same range applies to strings and numbers. The server rejects fractions and exponent notation."}
	id := map[string]any{"type": "string", "pattern": "^(?:[0-9a-fA-F]{32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$"}
	attributes := rpcArray(rpcObject(map[string]any{"id": text, "key": text, "value": rpcSchemaRef("Value")}, "id", "key", "value"))
	nullableText := map[string]any{"type": []string{"string", "null"}}
	nullableInteger := map[string]any{"type": []string{"integer", "null"}}
	nullableObject := map[string]any{"type": []string{"object", "null"}}
	attributePath := rpcArray(map[string]any{"oneOf": []any{text, map[string]any{"type": "integer", "minimum": 0, "maximum": 4294967294}}})
	rootSpan := rpcObject(map[string]any{"name": text, "serviceName": nullableText}, "name", "serviceName")
	rootSpan["type"] = []string{"object", "null"}
	uintText := map[string]any{"type": "string", "pattern": "^[0-9]+$"}
	seriesTime := map[string]any{"anyOf": []any{uintText, map[string]any{"type": "null"}}}
	seriesEntry := rpcObject(map[string]any{
		"seriesRef": rpcSchemaRef("EntityID"), "attributes": rpcSchemaRef("Attributes"), "datapointCount": uintText,
		"firstDatapointTimestamp": seriesTime, "lastDatapointTimestamp": seriesTime,
	}, "seriesRef", "attributes", "datapointCount", "firstDatapointTimestamp", "lastDatapointTimestamp")
	schemas := map[string]any{
		"Decimal": decimal, "UintInput": uintInput, "IntInput": intInput, "EntityID": id,
		"TimeBound": map[string]any{"anyOf": []any{rpcSchemaRef("UintInput"), map[string]any{"type": "null"}}, "description": "Inclusive bound in Unix nanoseconds. Null means unbounded. Zero means the Unix epoch."},
		"Value": map[string]any{"oneOf": []any{
			rpcObject(map[string]any{"kind": map[string]any{"const": "empty"}, "value": map[string]any{"type": "null"}}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "string"}, "value": text}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "bool"}, "value": boolean}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "int64"}, "value": decimal}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "double"}, "value": rpcSchemaRef("Double")}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "bytes"}, "value": map[string]any{"type": "string", "contentEncoding": "base64"}}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "array"}, "value": rpcArray(rpcSchemaRef("Value"))}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "map"}, "value": rpcArray(rpcObject(map[string]any{"key": text, "value": rpcSchemaRef("Value")}, "key", "value"))}, "kind", "value"),
		}, "description": "Numeric values:\n- int64: exact signed decimal string.\n- double: JSON number, except for negative zero and non-finite values. Those use 0x followed by 16 IEEE-754 hex digits.\n\nCollections: arrays keep order and duplicates. Each map keeps the last received value for each key, then sorts the keys. Each owner keeps its own attributes."},
		"Double":               map[string]any{"oneOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "string", "pattern": "^0x[0-9a-f]{16}$"}}},
		"Attributes":           attributes,
		"Resource":             rpcObject(map[string]any{"attributes": attributes, "droppedAttributesCount": integer, "schemaUrl": text}, "attributes", "droppedAttributesCount"),
		"Scope":                rpcObject(map[string]any{"name": text, "version": text, "attributes": attributes, "droppedAttributesCount": integer, "schemaUrl": text}, "name", "version", "attributes", "droppedAttributesCount"),
		"AttributeDefinitions": rpcArray(rpcObject(map[string]any{"name": text, "type": text, "attributeScope": text, "owner": text, "path": attributePath}, "name", "type", "attributeScope", "owner", "path")),
		"AttributeMatches": rpcArray(rpcObject(map[string]any{
			"name": text, "type": text, "attributeScope": text, "owner": text, "path": attributePath,
			"matchCount":   map[string]any{"type": "integer", "minimum": 0, "description": "Number of distinct matching values."},
			"sampleValues": map[string]any{"type": "array", "items": rpcSchemaRef("Value"), "maxItems": 3},
		}, "name", "type", "attributeScope", "owner", "path", "matchCount", "sampleValues")),
		"ServiceLabels": rpcObject(map[string]any{"serviceName": text, "serviceNamespace": text}, "serviceName", "serviceNamespace"),
		"TraceSummaries": rpcArray(rpcObject(map[string]any{"traceID": text, "hasRootSpan": boolean, "rootSpan": rootSpan,
			"startTime": decimal, "durationNs": nullableText, "spanCount": integer, "errorCount": integer, "matchedSpans": rpcArray(object),
		}, "traceID", "hasRootSpan", "rootSpan", "startTime", "durationNs", "spanCount", "errorCount")),
		"LogSummaries": rpcArray(rpcObject(map[string]any{"logRef": text, "timestamp": map[string]any{"type": "string", "pattern": decimal["pattern"], "description": "Received timestamp in exact unsigned decimal Unix nanoseconds. If zero, uses observedTimestamp."}, "severityText": text, "severityNumber": integer,
			"serviceName": text, "bodyPreview": map[string]any{"type": "string", "description": "Display text. Use getLog for the full typed body."}, "bodyType": text, "spanID": nullableText, "eventName": text,
			"columnValues": rpcArray(rpcObject(map[string]any{
				"source": text, "kind": text, "key": text, "valueType": text,
				"present": map[string]any{"type": "boolean", "description": "False for a missing value; true for a received empty value."},
				"preview": map[string]any{"type": "string", "description": "Display text. Timestamp fields use decimal Unix nanoseconds; trace and span IDs use hex."},
			}, "source", "kind", "key", "present", "valueType", "preview")),
		}, "logRef", "timestamp", "severityText", "severityNumber", "serviceName", "bodyPreview")),
		"LogPage": rpcObject(map[string]any{"logs": rpcSchemaRef("LogSummaries"), "total": integer}, "logs", "total"),
		"LogColumn": rpcObject(map[string]any{
			"source": map[string]any{"enum": []string{"log", "resource", "scope"}},
			"kind":   map[string]any{"enum": []string{"field", "attribute"}}, "key": text,
		}, "source", "kind", "key"),
		"LogErrorNavigation": rpcObject(map[string]any{"count": integer, "index": nullableInteger, "previous": map[string]any{"anyOf": []any{rpcObject(map[string]any{"logRef": text, "offset": integer}, "logRef", "offset"), map[string]any{"type": "null"}}}, "next": map[string]any{"anyOf": []any{rpcObject(map[string]any{"logRef": text, "offset": integer}, "logRef", "offset"), map[string]any{"type": "null"}}}}, "count", "index", "previous", "next"),
		"MetricSummaries": rpcArray(rpcObject(map[string]any{"metricRef": text, "name": text, "unit": text, "metricType": text,
			"aggregationTemporalityCode": nullableInteger, "aggregationTemporality": nullableText, "isMonotonic": map[string]any{"type": []string{"boolean", "null"}},
			"serviceName": text, "seriesCount": nullableInteger, "seriesCardinality": integer, "dataPointCount": nullableInteger,
			"lastValue": map[string]any{"anyOf": []any{rpcSchemaRef("Double"), map[string]any{"type": "null"}}}, "lastSeen": nullableText,
		}, "metricRef", "name", "metricType", "seriesCount", "seriesCardinality", "dataPointCount", "lastValue", "lastSeen")),

		"SearchQuery": map[string]any{"anyOf": []any{map[string]any{"type": "null"}, rpcSchemaRef("SearchNode")}},
		"SearchNode": map[string]any{"description": "Signal-specific fields/operators. Path: owner-relative keys or array indices.", "oneOf": []any{
			rpcRecord(map[string]any{"type": map[string]any{"const": "condition"}, "query": rpcObject(map[string]any{"fieldOperator": map[string]any{"type": "string", "minLength": 1}, "field": object, "value": text}, "field", "fieldOperator")}),
			rpcRecord(map[string]any{"type": map[string]any{"const": "text"}, "query": rpcObject(map[string]any{"fieldOperator": map[string]any{"enum": []string{"CONTAINS", "NOT CONTAINS"}}, "field": map[string]any{"type": "null"}, "value": text}, "fieldOperator")}),
			rpcRecord(map[string]any{"type": map[string]any{"const": "group"}, "group": map[string]any{"type": "object", "properties": map[string]any{"logicalOperator": text, "children": rpcArray(rpcSchemaRef("SearchNode"))}}}),
		}},
		"Sort": map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "object", "required": []string{"field", "direction"}, "additionalProperties": false, "properties": map[string]any{"field": text, "direction": map[string]any{"enum": []string{"asc", "desc"}}}}}},
		"QueryResult": rpcObject(map[string]any{
			"columns": rpcArray(rpcObject(map[string]any{"name": text, "type": text}, "name", "type")),
			"rows":    rpcArray(rpcArray(map[string]any{})), "truncated": boolean,
		}, "columns", "rows", "truncated"),
		"Log": rpcObject(map[string]any{"logRef": text, "timestamp": decimal, "observedTimestamp": decimal,
			"traceID": map[string]any{"type": []string{"string", "null"}}, "spanID": map[string]any{"type": []string{"string", "null"}},
			"severityText": text, "severityNumber": integer, "body": rpcSchemaRef("Value"), "attributes": attributes,
			"resource": rpcSchemaRef("Resource"), "scope": rpcSchemaRef("Scope"), "resourceSchemaURL": text, "scopeSchemaURL": text,
			"droppedAttributesCount": integer, "flags": integer, "eventName": text,
		}, "logRef", "timestamp", "observedTimestamp", "traceID", "spanID", "body", "attributes", "resource", "scope"),
		"TraceView": rpcObject(map[string]any{"traceID": text, "traceStart": map[string]any{"type": []string{"string", "null"}},
			"resources": object, "scopes": object, "unplacedSpanCount": integer,
			"spans": rpcArray(rpcObject(map[string]any{"spanData": object, "depth": integer, "matched": boolean}, "spanData", "depth", "matched")),
		}, "traceID", "traceStart", "resources", "scopes", "unplacedSpanCount", "spans"),
		"TraceOverview": rpcObject(map[string]any{"trace": rpcObject(map[string]any{"traceID": text, "spanCount": integer, "logCount": integer, "startTime": decimal, "durationNs": decimal}, "traceID", "spanCount", "logCount", "startTime", "durationNs"), "spans": rpcArray(object), "logs": rpcArray(object)}, "trace", "spans", "logs"),
		"SpanResult": rpcObject(map[string]any{"status": map[string]any{"enum": []string{"found", "notFound", "ambiguous"}},
			"traceID": map[string]any{"type": []string{"string", "null"}}, "spanID": text,
			"span": object, "logs": rpcArray(rpcSchemaRef("Log")), "matchCount": integer, "summaries": rpcArray(object), "truncated": boolean,
		}, "status"),
		"Metric":       rpcObject(map[string]any{"metricRef": text, "name": text, "description": text, "unit": text, "metricType": text, "metadata": rpcSchemaRef("Attributes"), "resource": object, "scope": object, "series": rpcArray(seriesEntry)}, "metricRef", "name", "description", "unit", "metricType", "metadata", "resource", "scope", "series"),
		"MetricSeries": rpcObject(map[string]any{"metricRef": text, "seriesRef": text, "name": text, "description": text, "unit": text, "metricType": text, "metadata": attributes, "resource": object, "scope": object, "attributes": attributes, "datapoints": rpcArray(object)}, "metricRef", "seriesRef", "metricType", "datapoints", "attributes"),
		"MetricView": rpcObject(map[string]any{"metricRef": text, "name": text, "description": text, "unit": text, "metricType": text,
			"metadata": attributes, "resource": rpcSchemaRef("Resource"), "scope": rpcSchemaRef("Scope"),
			"timeseries": rpcArray(object), "aggregate": map[string]any{"type": []string{"array", "null"}, "items": object},
			"scalarAggregate": rpcObject(map[string]any{"selected": rpcArray(object), "all": rpcArray(object)}, "selected", "all"),
			"lastSeenNs":      nullableText, "datapointCount": integer, "boundsMismatch": nullableObject, "window": object,
		}, "metricRef", "metricType", "timeseries", "aggregate", "scalarAggregate", "lastSeenNs", "datapointCount", "boundsMismatch", "window"),
		"MetricAggregateView": rpcObject(map[string]any{"aggregate": map[string]any{"type": []string{"array", "null"}, "items": object},
			"scalarAggregate": rpcObject(map[string]any{"selected": rpcArray(object), "all": rpcArray(object)}, "selected", "all"),
		}, "aggregate", "scalarAggregate"),
		"Stats": rpcObject(map[string]any{"storage": rpcObject(map[string]any{"sizeBytes": integer, "maxSizeBytes": integer}, "sizeBytes", "maxSizeBytes"),
			"traces":     rpcObject(map[string]any{"traceCount": integer, "spanCount": integer, "serviceCount": integer, "errorCount": integer, "lastReceived": nullableText}, "traceCount", "spanCount", "serviceCount", "errorCount", "lastReceived"),
			"logs":       rpcObject(map[string]any{"logCount": integer, "errorCount": integer, "lastReceived": nullableText}, "logCount", "errorCount", "lastReceived"),
			"metrics":    rpcObject(map[string]any{"metricCount": integer, "dataPointCount": integer, "lastReceived": nullableText}, "metricCount", "dataPointCount", "lastReceived"),
			"rejections": rpcArray(object),
		}, "storage", "traces", "logs", "metrics", "rejections"),
		"HomeCounts": rpcObject(map[string]any{"traces": integer, "logs": integer, "metrics": integer}, "traces", "logs", "metrics"),
		"HomeValue": rpcObject(map[string]any{
			"preview": text, "identity": text, "valueRef": text, "kind": text, "colorHash": integer,
			"counts": rpcSchemaRef("HomeCounts"), "count": integer, "kindCount": integer, "denominator": integer,
			"relativeFrequency": map[string]any{"type": "number"}, "foundOn": rpcArray(rpcObject(map[string]any{"signal": text, "ownerType": text}, "signal", "ownerType")),
		}, "preview", "identity", "valueRef", "kind", "colorHash", "counts", "count", "kindCount", "denominator", "relativeFrequency", "foundOn"),
		"DiscoveryResult": map[string]any{"oneOf": []any{
			rpcSchemaRef("DiscoveryRows"),
			rpcObject(map[string]any{
				"values": rpcArray(rpcSchemaRef("HomeValue")), "kinds": rpcArray(rpcObject(map[string]any{"kind": text, "count": integer, "counts": rpcSchemaRef("HomeCounts")}, "kind", "count", "counts")),
				"denominators": rpcSchemaRef("HomeCounts"), "denominator": integer, "total": integer, "truncated": boolean,
			}, "values", "kinds", "denominators", "denominator", "total", "truncated"),
			rpcObject(map[string]any{"identity": text}, "identity"),
		}},
		"StringList":   rpcArray(text),
		"DeleteResult": rpcObject(map[string]any{"message": text, "count": integer}, "message", "count"),
		"ImportConfig": rpcObject(map[string]any{"otlpHttpPort": integer, "otlpGrpcPort": integer}, "otlpHttpPort"),
		"DatabaseEntry": rpcObject(map[string]any{
			"name": text, "sizeBytes": integer, "modifiedAt": text, "active": boolean,
		}, "name", "sizeBytes", "modifiedAt", "active"),
		"DatabaseList": rpcObject(map[string]any{
			"managed": boolean, "active": text, "databases": rpcArray(rpcSchemaRef("DatabaseEntry")),
		}, "managed", "active", "databases"),
		"Integer": integer, "String": text,
	}
	count := map[string]any{"type": "integer", "minimum": 0}
	schemas["DiscoveryService"] = rpcRecord(map[string]any{
		"serviceIdentity": text, "serviceColorKey": text, "serviceName": text, "serviceNamespace": text, "serviceNameKind": text, "serviceNamespaceKind": text,
		"labelsComplete": boolean, "traceCount": count, "spanCount": count, "errorSpanCount": count, "logCount": count, "errorLogCount": count,
		"metricCount": count, "dataPointCount": count, "exemplarCount": count, "lastSeen": seriesTime,
		"logSeverityCounts": rpcRecord(map[string]any{"trace": count, "debug": count, "info": count, "warn": count, "error": count, "fatal": count, "other": count}),
	})
	schemas["DiscoveryRows"] = rpcDiscoveryRows(rpcSchemaRef("DiscoveryService"), rpcRecord(map[string]any{"signal": text, "ownerType": text}), text, count, uintText)
	specs := []struct {
		name, result, summary, description string
		required                           int
	}{
		{"rpc.discover", "", "Retrieve the full OpenRPC document over JSON-RPC.", "Equivalent to GET /openrpc.json. Does not require database access.", 0},
		{"getImportConfig", "ImportConfig", "Find the OTLP HTTP port used for import.", "If HTTP import is not configured, the server returns -32603.", 0},
		{"query", "QueryResult", "Run read-only DuckDB SQL against stored telemetry.", "Each row follows column order. SQL null becomes JSON null. truncated is true if another row exists. For exact integers, decimals and numbers inside JSON columns, use a parser that preserves precision or cast to varchar in SQL. Top-level float and double values use JSON numbers when finite and OTLP strings otherwise.", 1},

		{"discoverTelemetry", "DiscoveryResult", "Find services, connections, spans and attribute values.", "services → connections: reuse serviceIdentity. connections → connection-spans: pass source.serviceIdentity and target.serviceIdentity.", 1},
		{"searchTraceSummaries", "TraceSummaries", "Find matching traces and their IDs.", "Time and query filters select spans. matchedSpans identifies the matches. Counts and time bounds cover the whole retained trace. durationNs is the latest end minus the earliest start, in exact decimal nanoseconds. There is no truncation flag. Request one extra row to detect more matches.", 2},
		{"getTraceView", "TraceView", "UI view: Read the waterfall hierarchy and search highlights.", "Use spanData.r and spanData.s as keys into resources and scopes. traceStart is the earliest span start. spanData.start is relative to traceStart. dur is end minus start. Both use exact decimal nanoseconds. Replace the whole response when refreshing: offsets share one baseline.\n\nThe viewer computes depth, matched and the kind and status labels. kindCode and statusCodeValue preserve the received signed codes. query highlights matches.", 1},
		{"getTraceOverview", "TraceOverview", "List a trace’s spans and log previews.", "trace.startTime is the earliest span start. Trace durationNs is the latest end minus that start. Each span’s startOffsetNs is relative to trace.startTime. Its durationNs is end minus start. All times use exact decimal nanoseconds. Counts cover retained records.\n\nlogs.body is a preview. Severity uses received text or a label derived from the severity number. Use getSpan or getLog for the full record.", 1},
		{"getSpan", "SpanResult", "Read a span, its events, links, and matching logs by trace ID and span ID.", "Supply traceID when known. A spanID alone can match more than one trace. Check status:\n\n- found: traceID, span and logs. Span times are exact decimal Unix nanoseconds. Logs include typed bodies.\n- notFound: spanID and nullable traceID.\n- ambiguous: spanID, matchCount, summaries and truncated.", 1},
		{"searchLogSummaries", "LogSummaries", "Find matching logs and their logRef values.", "There is no truncation flag. Use getLog for the full typed body.", 2},
		{"searchLogPage", "LogPage", "Read a page of matching logs and the total match count.", "The page and total come from the same read. Later requests can see newly received or deleted logs.", 2},
		{"getLogErrorNavigation", "LogErrorNavigation", "Find the previous or next error in a log search.", "Filters, sort and limit apply before navigation. Errors have severityNumber >= 17. Targets use zero-based result offsets. index is the selected error’s one-based position, or null for a missing or non-error selection. Navigation does not wrap. For a missing selection, previous is null and next points to the first error, if any. Counts and targets come from the same read.", 2},
		{"getTraceLogSummaries", "LogSummaries", "Find logs carrying a given trace ID.", "Includes logs with no spanID.", 1},
		{"getLog", "Log", "Read the full typed body and context for one logRef.", "timestamp and observedTimestamp are separate received times in unsigned decimal Unix nanoseconds. Missing trace and span IDs are null. The log includes its resource, scope, schema URLs, typed body and typed attributes.", 1},
		{"searchMetricSummaries", "MetricSummaries", "Find matching Metrics and their metricRef values.", "Counts: seriesCount and dataPointCount cover the search window. seriesCardinality covers all retained datapoints. There is no truncation flag.\n\nValues: lastSeen is the latest datapoint timestamp in the window, in decimal Unix nanoseconds. lastValue is the latest scalar for display and can round int64 to double. aggregationTemporalityCode is received. Its label is computed.\n\nUse getMetric for metadata and series references. Use getMetricSeries for exact datapoints.", 2},
		{"getMetric", "Metric", "Read a Metric descriptor and find its series references.", "The catalogue uses database-local series references. Metadata and attributes retain their types. datapointCount is an exact decimal string. firstDatapointTimestamp and lastDatapointTimestamp are the earliest and latest received timestamps, or null for an empty series.\n\nTemporality and isMonotonic appear only for applicable Metric kinds. Unknown signed temporality codes retain their received values.", 1},
		{"getMetricSeries", "MetricSeries", "Read exact datapoints and exemplars for one Metric series.", "Each datapoint includes its own exemplars and typed attributes. An empty window returns datapoints: [].\n\n- Decimal strings: timestamps, startTime, signed intValue and unsigned count, bucketCounts and zeroCount.\n- Doubles: JSON numbers except for negative zero and non-finite values, which use IEEE-754 bit strings.\n- Missing values: the inactive intValue or doubleValue is null. Absent histogram sum, min and max are omitted.", 4},
		{"getMetricView", "MetricView", "UI view: Read Metric charts, statistics, and sparklines.", "Chart calculations can use floating point. Received numeric fields keep their exact encoding. See docs/metric-charts.md for bucket and aggregation formulas, units and precision.\n\nSeries have empty attributes and attributesComplete=false. labelPreviews contains short display labels. Use getMetricSeriesOrder to sort series by attribute. Use getMetricSeries for full attributes and exact datapoints.", 3},
		{"getMetricAggregateView", "MetricAggregateView", "UI view: Read the combined chart across Metric series.", "Returns only the computed aggregate chart. Use getMetricView for per-series data. See docs/metric-charts.md for formulas, units and precision.", 3},
		{"getMetricSeriesOrder", "StringList", "Get series references sorted by an attribute.", "Returns each requested series once if it belongs to the Metric and has datapoints in the window. Treat an incomplete list as stale.\n\nSort order:\n- Numbers precede other values in both directions. Integer and double comparisons are exact. In ascending order, NaN follows +Infinity.\n- Other values use English natural text order and typed signatures. A missing attribute differs from an empty value.\n- Ties use ascending series ID in both directions.", 6},
		{"clearTraces", "String", "Delete all retained traces and their spans.", "Also removes unused shared dictionary entries.", 0},
		{"clearLogs", "String", "Delete all retained logs.", "Also removes unused shared dictionary entries.", 0},
		{"clearMetrics", "String", "Delete all retained Metrics.", "Also removes unused shared dictionary entries.", 0},
		{"deleteMetric", "String", "Delete one Metric and its dependent records.", "On success, returns the text 'Metric deleted successfully'.", 1},
		{"deleteSpansByTraceID", "DeleteResult", "Delete all spans in the specified traces.", "Variadic positional parameters: pass one trace ID per position, as [id1, id2]. Named parameters and nested arrays are invalid. count measures supplied IDs, not deleted spans.", 1},
		{"deleteLogsByRefs", "DeleteResult", "Delete the logs identified by the supplied logRef values.", "Variadic positional parameters: pass one log reference per position, as [ref1, ref2]. Named parameters and nested arrays are invalid. count measures supplied references, not deleted logs.", 1},
		{"getTraceAttributeDefinitions", "AttributeDefinitions", "List attribute keys, types, and scopes available to trace filters.", "Includes all retained times.", 0},
		{"getLogAttributeDefinitions", "AttributeDefinitions", "List attribute keys, types, and scopes available to log filters.", "Includes all retained times.", 0},
		{"getMetricAttributeDefinitions", "AttributeDefinitions", "List attribute keys, types, and scopes available to Metric filters.", "Includes all retained times.", 0},
		{"getTraceAttributeDefinitionsByTraceID", "AttributeDefinitions", "List filter attributes for one trace.", "Use getTraceAttributeDefinitions for attributes across all retained traces.", 1},
		{"getFieldValueCompletions", "StringList", "Suggest text completions for a search field.", "Returns distinct display strings. An unknown signal returns -32602. An unsupported field returns -32007.", 4},
		{"searchAttributeMatches", "AttributeMatches", "Find attribute keys or sample typed values at a selected path, without time bounds.", "Search by key name first. Supply owner and path to search values and receive typed samples. Without a path, sampleValues is empty.", 1},
		{"getStats", "Stats", "Check retained record counts, storage use, and ingestion rejections.", "Counts measure retained records. maxSizeBytes=0 disables the retention cap.\n\nlastReceived uses source timestamps in decimal Unix nanoseconds, or null. It does not measure arrival time:\n- Traces: latest span start.\n- Logs: latest nonzero timestamp. Uses the latest observed timestamp only if all timestamps are zero.\n- Metrics: latest datapoint timestamp.", 0},
		{"getTraceSpanCount", "Integer", "Count stored spans in one trace.", "Returns 0 if the trace has no stored spans.", 1},
		{"getServiceCount", "Integer", "Count distinct services across the database.", "Counts typed service identities across retained spans, logs and Metric datapoints. A missing namespace counts as an empty namespace. A missing service name stays distinct.", 0},
		{"getServiceLabels", "ServiceLabels", "Resolve a service reference to its full name and namespace.", "Pass the serviceIdentity from a service summary as serviceRef to get unabridged labels.", 1},
		{"listDatabases", "DatabaseList", "List managed session databases and report the active capture.", "In external --db mode, managed is false and databases is empty.", 0},
		{"openDatabase", "DatabaseList", "Switch the viewer to an existing managed session database.", "Checkpoints and closes the active capture before opening the selected .duckdb file.", 1},
		{"newDatabase", "DatabaseList", "Create a new managed session database and switch collection to it.", "Appends .duckdb when omitted and refuses to overwrite an existing file.", 1},
	}
	methods := make([]rpcMethodDescription, 0, len(specs))
	for _, spec := range specs {
		method := rpcMethodDescription{Name: spec.name, Summary: spec.summary, Description: spec.description, ParamStructure: "either", Params: []rpcDescriptor{}, Result: rpcDescriptor{Name: "result", Required: true, Schema: rpcSchemaRef(spec.result)}}
		if spec.name == "getSpan" || spec.name == "getTraceView" {
			method.Result.Description = "link.summary.target contains the stored target’s name and serviceName, or null if the traceID and spanID pair is missing. OTLP export excludes this computed summary."
		}
		if spec.name == "rpc.discover" {
			method.Result.Schema = map[string]any{"$ref": openRPCMetaSchema}
		} else if spec.name != "getImportConfig" {
			method.Errors = []map[string]any{{"code": ErrCodeRequestCanceled, "message": "Request canceled"}}
		}
		if spec.name == "discoverTelemetry" || spec.name == "getServiceLabels" {
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeHomeValueUnavailable, "message": ErrHomeValueUnavailable.Error()})
		}
		names := methodParamNames[spec.name]
		if spec.name == "deleteSpansByTraceID" || spec.name == "deleteLogsByRefs" {
			method.ParamStructure = "by-position"
			names = []string{"traceID"}
			if spec.name == "deleteLogsByRefs" {
				names = []string{"logRef"}
			}
		}
		for i, name := range names {
			schema := rpcSchemaRef("String")
			description := ""
			switch name {
			case "options":
				schema = rpcDiscoveryOptions()
			case "traceID", "logRef", "metricRef", "seriesRef", "selectedLogRef":
				schema = rpcSchemaRef("EntityID")
				description = "Received trace ID (32 hex or dashed UUID)."
				if name != "traceID" {
					description = "Generated database-local reference from this viewer, not a received OTel identifier."
				}
			case "spanID":
				schema = map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{16}$"}
			case "startTime", "endTime":
				schema = rpcSchemaRef("TimeBound")
				description = "Pass null for no bound."
			case "query":
				schema = rpcSchemaRef("SearchQuery")
			case "sort":
				schema = rpcSchemaRef("Sort")
				if spec.name == "searchLogPage" {
					description = "Sort matches before paging. Ties use logRef in ascending order."
				}
			case "pageSize":
				schema = rpcSchemaRef("IntInput")
				description = "Rows per page: 1 through 250. Omit or pass null for 100."
			case "offset":
				schema = rpcSchemaRef("IntInput")
				description = "Number of matches to skip. Minimum: 0. Omit or pass null for 0. At or beyond total, logs is empty and total stays the same."
			case "columns":
				schema = rpcArray(rpcSchemaRef("LogColumn"))
				description = "Select each source/kind/key once. columnValues follows request order. Omit columns or pass [] for summaries alone. Attribute keys may be empty.\n\nAllowed fields by source:\n- log: timeUnixNano, observedTimeUnixNano, severityText, severityNumber, traceId, spanId, eventName, body, flags, droppedAttributesCount.\n- resource: schemaUrl, droppedAttributesCount.\n- scope: name, version, schemaUrl, droppedAttributesCount."
			case "limit":
				schema = rpcSchemaRef("IntInput")
				description = "Positive int64. Omit or pass null for no limit."
				if spec.name == "query" {
					schema = map[string]any{"type": "integer", "minimum": 0, "maximum": json.Number("18446744073709551615"), "default": 25}
					description = "Maximum rows. Default: 25. Use a uint64 JSON number. Zero returns no rows. Strings and null are invalid."
				} else if spec.name == "searchLogPage" {
					description = "Maximum matches before paging. Also caps total. Use a positive int64. Omit or pass null for no limit."
				} else if spec.name == "getSpan" {
					description = "Maximum summaries for an ambiguous span ID: 1 through 9223372036854775806. Omit or pass null for 25."
				} else if spec.name == "getFieldValueCompletions" {
					description = "Signed int64. Values below 1 use 1. Values above 500 use 500. Null is invalid."
				}
			case "targetBuckets", "viewBuckets", "sparklineBuckets", "datapointSeriesLimit", "tzOffsetNs":
				schema = rpcSchemaRef("IntInput")
				switch name {
				case "targetBuckets":
					description = "Number of time buckets for datapoint reduction. Omit, pass null or use 0 for no reduction."
				case "viewBuckets":
					description = "Number of time buckets for chart calculations. Minimum: 0. Omit or pass null for 0."
				case "sparklineBuckets":
					description = "Number of buckets per sparkline. Omit, pass null or use 0 to skip sparklines."
				case "datapointSeriesLimit":
					description = "Maximum series that include datapoints, in response order. Omit, pass null or use 0 for no cap."
				case "tzOffsetNs":
					description = "Time zone offset in signed nanoseconds. Omit or pass null for 0."
				}
			case "tzName":
				description = "Time zone name for chart calculations."
			case "seriesRefs", "selectedSeriesRefs", "datapointSeriesRefs":
				schema = rpcArray(text)
				switch {
				case spec.name == "getMetricSeriesOrder":
					description = "List the database-local series references to sort. An empty list returns []. Null is invalid."
				case name == "selectedSeriesRefs":
					description = "Series to combine in the selected aggregate. Omit or pass null for no selection."
				case name == "datapointSeriesRefs":
					description = "Series whose datapoints to return. An empty list returns no datapoints. Omit or pass null to use datapointSeriesLimit. Other series keep their statistics and chart summaries."
				default:
					description = "Series to include. Omit or pass null for all series. An empty list selects none."
				}
			case "key":
				description = "Exact attribute key to sort by."
			case "direction":
				schema = map[string]any{"type": "string", "enum": []string{"asc", "desc"}}
			case "quantiles":
				schema = rpcArray(map[string]any{"type": "number", "minimum": 0, "maximum": 1})
			case "includeDatapoints":
				schema = map[string]any{"type": []string{"boolean", "null"}, "default": true}
				description = "Pass false for full series context with datapoints: []. Omit or pass null to include datapoints."
			case "term":
				if spec.name == "getFieldValueCompletions" {
					description = "Case-insensitive literal substring. An empty string matches all values."
				}
			case "spanIDs":
				schema = map[string]any{"type": "array", "minItems": 1, "maxItems": 256, "items": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{16}$"}}
				description = "Return only the selected stored spans and their attributes, events, links, resources and scopes. Depth uses available ancestors. parentSpanID keeps its received value. Omit or pass null for the whole trace. An empty list is invalid."
			case "signal":
				schema = map[string]any{"enum": []string{"traces", "logs", "metrics"}}
			case "owner":
				schema = map[string]any{"enum": []string{"span", "event", "link", "log", "metric", "datapoint", "exemplar", "resource", "scope"}}
			case "path":
				schema = attributePath
			case "type":
				schema = map[string]any{"enum": []string{"string", "int64", "double", "bool", "bytes", "empty", "map", "array"}}
			case "field":
				schema = map[string]any{"enum": []string{"name", "eventName", "unit"}}
				description = "Allowed pairs: traces/name, logs/eventName, metrics/name or metrics/unit."
			case "sql":
				description = "One read-only select or supported inspection statement. Use show tables to list tables and describe <table> to list columns."
			case "name":
				schema = map[string]any{"type": "string", "minLength": 1}
				if spec.name == "newDatabase" {
					description = "Plain session filename ('.duckdb' is appended when omitted). Cannot contain path separators, '%', '?', control characters, or Windows-illegal characters."
				} else {
					description = "Existing managed session filename ending in '.duckdb'."
				}
			}
			if i >= spec.required && spec.name != "query" && name != "query" && name != "sort" {
				schema = map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
			}
			method.Params = append(method.Params, rpcDescriptor{Name: name, Description: description, Required: i < spec.required, Schema: schema})
			if name == "traceID" {
				method.Errors = append(method.Errors, map[string]any{"code": ErrCodeInvalidTraceID, "message": "Invalid trace ID"})
			} else if name == "logRef" {
				method.Errors = append(method.Errors, map[string]any{"code": ErrCodeInvalidLogRef, "message": "Invalid log reference"})
			} else if name == "metricRef" {
				method.Errors = append(method.Errors, map[string]any{"code": ErrCodeInvalidMetricRef, "message": "Invalid Metric reference"})
			}
		}
		switch spec.name {
		case "getTraceView", "getTraceOverview":
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeTraceNotFound, "message": "Trace not found"})
		case "getLog":
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeLogNotFound, "message": "Log not found"})
		case "getMetric", "getMetricSeries", "getMetricView", "getMetricAggregateView":
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeMetricNotFound, "message": "Metric not found"})
		case "openDatabase":
			method.Errors = append(method.Errors,
				map[string]any{"code": ErrCodeDatabaseNotFound, "message": "Database not found"},
				map[string]any{"code": ErrCodeDatabaseSwitchUnsupported, "message": ErrDatabaseSwitchUnsupported.Error()},
				map[string]any{"code": ErrCodeDatabaseIncompatible, "message": ErrDatabaseIncompatible.Error()},
			)
		case "newDatabase":
			method.Errors = append(method.Errors,
				map[string]any{"code": ErrCodeDatabaseAlreadyExists, "message": "Database already exists"},
				map[string]any{"code": ErrCodeDatabaseSwitchUnsupported, "message": ErrDatabaseSwitchUnsupported.Error()},
			)
		}
		if spec.name == "query" || spec.name == "getTraceView" || spec.name == "searchTraceSummaries" || spec.name == "searchLogSummaries" || spec.name == "searchLogPage" || spec.name == "getLogErrorNavigation" || spec.name == "searchMetricSummaries" || spec.name == "getFieldValueCompletions" {
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeInvalidQuery, "message": "Invalid query"})
		}
		method.Examples = rpcExamples(spec.name)
		methods = append(methods, method)
	}
	return map[string]any{
		"openrpc": "1.3.2", "$schema": openRPCMetaSchema,
		"info":    map[string]any{"title": "OTel Desktop Viewer JSON-RPC", "version": "1.0.0", "description": "POST JSON-RPC 2.0 requests to /rpc. Responses use application/json and preserve the request id.\n\nParameters: required marks positional slots. With named parameters, skipped slots before the last supplied parameter become null. Required trailing slots must still be supplied. Unknown names are invalid.\n\nStandard errors: -32700 parse error, -32600 invalid request, -32601 unknown method, -32602 invalid params, -32603 internal error.\n\nMethods ending in View return UI-shaped data intended for rendering in the viewer."},
		"servers": []map[string]any{{"name": "viewer", "url": "/rpc"}},
		"methods": methods, "components": map[string]any{"schemas": schemas},
	}
}

func rpcDiscoveryOptions() map[string]any {
	text := map[string]any{"type": "string"}
	nullableText := map[string]any{"type": []string{"string", "null"}}
	bound := map[string]any{"type": []string{"string", "null"}, "pattern": "^[0-9]+$"}
	choice := func(values ...string) map[string]any { return map[string]any{"enum": values} }
	limit := func(min, max, fallback int) map[string]any {
		return map[string]any{"type": []string{"integer", "null"}, "minimum": min, "maximum": max, "default": fallback}
	}
	only := func(kind string) map[string]any {
		return map[string]any{"properties": map[string]any{"kind": map[string]any{"const": kind}}}
	}
	mode := func(kind, description string, fields ...string) map[string]any {
		props := map[string]any{"kind": map[string]any{"const": kind}}
		s := map[string]any{"properties": props, "description": description}
		if len(fields) > 0 {
			s["required"] = fields
		}
		switch kind {
		case "services":
			s["propertyNames"] = map[string]any{"not": choice("ownerType", "key")}
		case "connections", "connection-spans":
			props["serviceIdentity"] = text
			s["propertyNames"] = map[string]any{"not": choice("ownerType", "key", "serviceName", "namespace")}
		case "keys":
			s["propertyNames"] = map[string]any{"not": choice("key")}
		case "home-values":
			s["propertyNames"] = map[string]any{"not": choice("signal")}
			props["limit"] = limit(0, 100, 25)
		case "home-value":
			s["maxProperties"] = 2 // Only kind and valueRef.
		}
		return s
	}
	return map[string]any{
		"type": "object", "required": []string{"kind"}, "additionalProperties": false,
		"description": "Times: inclusive uint64 Unix nanosecond strings or null. Exact service filters distinguish empty from omitted. term: case-insensitive substring. Per signal: null selects all, false none. Filters precede paging.",
		"if":          only("home-values"),
		"else":        map[string]any{"properties": map[string]any{"limit": limit(1, 500, 100)}},
		"properties": map[string]any{
			"kind": map[string]any{}, "signal": text, "limit": map[string]any{},
			"startTime": bound, "endTime": bound, "offset": limit(0, 1_000_000_000, 0),
			"key": text, "targetIdentity": text,
			"term": nullableText, "serviceName": nullableText, "namespace": nullableText, "serviceIdentity": nullableText,
			"valueKind": nullableText, "locale": nullableText, "valueRef": map[string]any{"type": "string", "minLength": 1},
			"sortBy": map[string]any{"enum": []any{nil, "", "name", "frequency"}}, "sortDirection": map[string]any{"enum": []any{nil, "", "asc", "desc"}},
			"ownerType": choice("span", "event", "link", "log", "datapoint", "exemplar", "metadata", "resource", "scope"),
			"queries": map[string]any{"type": []string{"object", "null"}, "propertyNames": choice("traces", "logs", "metrics"),
				"additionalProperties": map[string]any{"anyOf": []any{rpcSchemaRef("SearchQuery"), map[string]any{"const": false}}}},
		},
		"dependencies": map[string]any{
			"targetIdentity": only("connection-spans"), "valueRef": only("home-value"),
			"locale": only("home-values"), "valueKind": only("home-values"),
			"sortBy": []string{"key"}, "sortDirection": []string{"key"},
			"signal": map[string]any{"required": []string{"ownerType"}, "oneOf": []any{
				map[string]any{"properties": map[string]any{"signal": choice("traces"), "ownerType": choice("span", "event", "link", "resource", "scope")}},
				map[string]any{"properties": map[string]any{"signal": choice("logs"), "ownerType": choice("log", "resource", "scope")}},
				map[string]any{"properties": map[string]any{"signal": choice("metrics"), "ownerType": choice("datapoint", "exemplar", "metadata", "resource", "scope")}},
			}}},
		"oneOf": []any{
			mode("services", "Group by typed namespace/name; missing namespace counts as empty. Only totals.home includes service filters and term."),
			mode("connections", "Edges and counts. Both spans match one trace and window.", "serviceIdentity"),
			mode("connection-spans", "Child spans from source to target.", "serviceIdentity", "targetIdentity"),
			mode("keys", "Attribute keys and types.", "signal", "ownerType"),
			mode("values", "Counts distinct records per typed value. Denominator: records carrying key. Empty keys allowed.", "signal", "ownerType", "key"),
			mode("home-values", "Display preview; identify rows by identity. Limit 0: counts only. Otherwise advance by values returned.", "ownerType", "key"),
			mode("home-value", "Canonical identity (Arrow: received value). Refresh expired refs with home-values.", "valueRef"),
		},
	}
}

func rpcExamples(method string) []map[string]any {
	var params []map[string]any
	var result any
	switch method {
	case "discoverTelemetry":
		params = []map[string]any{{"name": "options", "value": map[string]any{"kind": "services"}}}
		result = map[string]any{"columns": []any{map[string]any{"name": "entry", "type": "JSON"}}, "rows": []any{}, "truncated": false}
	case "query":
		params = []map[string]any{{"name": "sql", "value": "select 9007199254740993::bigint as exact"}}
		result = map[string]any{"columns": []map[string]any{{"name": "exact", "type": "BIGINT"}}, "rows": [][]any{{json.Number("9007199254740993")}}, "truncated": false}
	case "searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries":
		params = []map[string]any{{"name": "startTime", "value": "18446744073709551615"}, {"name": "endTime", "value": "0"}}
		result = []any{}
	case "getSpan":
		params = []map[string]any{{"name": "spanID", "value": "0000000000000001"}, {"name": "traceID", "value": "00000000000000000000000000000001"}}
		result = map[string]any{"status": "notFound", "spanID": "0000000000000001", "traceID": "00000000000000000000000000000001"}
	default:
		return nil
	}
	return []map[string]any{{"name": "empty-store", "params": params, "result": map[string]any{"name": "result", "value": result}}}
}

func registerOpenRPC(mux *http.ServeMux) error {
	doc := openRPCDocument()
	full, err := apiDocumentHandler("openrpc.json", doc)
	if err != nil {
		return err
	}
	mux.Handle("GET /openrpc.json", full)
	all := maps.Clone(doc["components"].(map[string]any)["schemas"].(map[string]any))
	all["DiscoveryRows"] = map[string]any{"$ref": "/openrpc.json#/components/schemas/DiscoveryRows"}
	var index []map[string]string
	for _, method := range doc["methods"].([]rpcMethodDescription) {
		index = append(index, map[string]string{"name": method.Name, "summary": method.Summary})
		schemas := map[string]any{}
		for _, descriptor := range append([]rpcDescriptor{method.Result}, method.Params...) {
			if err := collectRPCSchemas(descriptor.Schema, all, schemas); err != nil {
				return fmt.Errorf("%s: %w", method.Name, err)
			}
		}
		single := maps.Clone(doc)
		info := maps.Clone(doc["info"].(map[string]any))
		delete(info, "description")
		single["info"] = info
		single["methods"] = []rpcMethodDescription{method}
		single["components"] = map[string]any{"schemas": schemas}
		filename := method.Name + ".json"
		handler, err := apiDocumentHandler(filename, single)
		if err != nil {
			return err
		}
		mux.Handle("GET /openrpc/"+filename, handler)
	}
	menu, err := apiDocumentHandler("index.json", map[string]any{"methods": index})
	if err != nil {
		return err
	}
	mux.Handle("GET /openrpc/index.json", menu)
	return nil
}

// Copy local dependencies once, including schemas that reference themselves.
func collectRPCSchemas(node any, all, selected map[string]any) error {
	switch v := node.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			if name, local := strings.CutPrefix(ref, "#/components/schemas/"); local {
				if _, visited := selected[name]; !visited {
					schema, exists := all[name]
					if !exists {
						return fmt.Errorf("unknown OpenRPC schema %q", name)
					}
					selected[name] = schema
					if err := collectRPCSchemas(schema, all, selected); err != nil {
						return err
					}
				}
			}
		}
		for _, child := range v {
			if err := collectRPCSchemas(child, all, selected); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := collectRPCSchemas(child, all, selected); err != nil {
				return err
			}
		}
	}
	return nil
}

func apiDocumentHandler(name string, value map[string]any) (http.Handler, error) {
	document, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(document)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", revalidateCacheControl)
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(document))
	}), nil
}
