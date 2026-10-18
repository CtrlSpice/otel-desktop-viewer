package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
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

func openRPCDocument() map[string]any {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	object := map[string]any{"type": "object"}
	decimal := map[string]any{"type": "string", "pattern": "^-?[0-9]+$", "description": "Exact decimal integer text. Never decode through a floating-point number."}
	uintInput := map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "pattern": "^[0-9]+$"},
		map[string]any{"type": "integer", "minimum": 0, "maximum": json.Number("18446744073709551615")},
	}, "description": "Unsigned 64-bit whole number, 0 through 18446744073709551615. Decimal strings are recommended; numeric tokens must be emitted without prior floating-point rounding. Fractional/exponent notation is rejected by the server."}
	intInput := map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "pattern": "^[+-]?[0-9]+$"},
		map[string]any{"type": "integer", "minimum": json.Number("-9223372036854775808"), "maximum": json.Number("9223372036854775807")},
	}, "description": "Signed 64-bit whole number; decimal strings recommended. String values must also fit int64. Fractional/exponent notation is rejected."}
	id := map[string]any{"type": "string", "pattern": "^(?:[0-9a-fA-F]{32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$"}
	attributes := rpcArray(rpcObject(map[string]any{"id": text, "key": text, "value": rpcSchemaRef("Value")}, "id", "key", "value"))
	nullableText := map[string]any{"type": []string{"string", "null"}}
	nullableInteger := map[string]any{"type": []string{"integer", "null"}}
	nullableObject := map[string]any{"type": []string{"object", "null"}}
	schemas := map[string]any{
		"Decimal": decimal, "UintInput": uintInput, "IntInput": intInput, "EntityID": id,
		"TimeBound": map[string]any{"anyOf": []any{rpcSchemaRef("UintInput"), map[string]any{"type": "null"}}, "description": "Inclusive received Unix nanoseconds. null means unbounded, not zero; 0 is a present bound."},
		"Value": map[string]any{"oneOf": []any{
			rpcObject(map[string]any{"kind": map[string]any{"const": "empty"}, "value": map[string]any{"type": "null"}}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "string"}, "value": text}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "bool"}, "value": boolean}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "int64"}, "value": decimal}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "double"}, "value": rpcSchemaRef("Double")}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "bytes"}, "value": map[string]any{"type": "string", "contentEncoding": "base64"}}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "array"}, "value": rpcArray(rpcSchemaRef("Value"))}, "kind", "value"),
			rpcObject(map[string]any{"kind": map[string]any{"const": "map"}, "value": rpcArray(rpcObject(map[string]any{"key": text, "value": rpcSchemaRef("Value")}, "key", "value"))}, "kind", "value"),
		}, "description": "Recursive received typed value. int64 uses exact signed decimal text; doubles use numbers or 0x followed by 16 IEEE-754 hex digits for negative zero/non-finite values. Arrays retain order/repetition. Maps select the last received occurrence per key within this map, then sort surviving keys. Owner collections are never combined."},
		"Double":               map[string]any{"oneOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "string", "pattern": "^0x[0-9a-f]{16}$"}}},
		"Attributes":           attributes,
		"Resource":             rpcObject(map[string]any{"attributes": attributes, "droppedAttributesCount": integer, "schemaUrl": text}, "attributes", "droppedAttributesCount"),
		"Scope":                rpcObject(map[string]any{"name": text, "version": text, "attributes": attributes, "droppedAttributesCount": integer, "schemaUrl": text}, "name", "version", "attributes", "droppedAttributesCount"),
		"AttributeDefinitions": rpcArray(rpcObject(map[string]any{"name": text, "type": text, "attributeScope": text}, "name", "type", "attributeScope")),
		"TraceSummaries": rpcArray(rpcObject(map[string]any{"traceID": text, "hasRootSpan": boolean, "rootSpan": nullableObject,
			"startTime": decimal, "durationNs": nullableText, "spanCount": integer, "errorCount": integer, "matchedSpans": rpcArray(object),
		}, "traceID", "hasRootSpan", "rootSpan", "startTime", "durationNs", "spanCount", "errorCount")),
		"LogSummaries": rpcArray(rpcObject(map[string]any{"logRef": text, "timestamp": decimal, "severityText": text, "severityNumber": integer,
			"serviceName": text, "bodyPreview": text, "spanID": nullableText, "eventName": text,
		}, "logRef", "timestamp", "severityText", "severityNumber", "serviceName", "bodyPreview")),
		"MetricSummaries": rpcArray(rpcObject(map[string]any{"metricRef": text, "name": text, "description": text, "unit": text, "metricType": text,
			"aggregationTemporalityCode": nullableInteger, "aggregationTemporality": nullableText, "isMonotonic": map[string]any{"type": []string{"boolean", "null"}},
			"serviceName": text, "seriesCount": nullableInteger, "seriesCardinality": integer, "dataPointCount": nullableInteger,
			"lastValue": map[string]any{"anyOf": []any{rpcSchemaRef("Double"), map[string]any{"type": "null"}}}, "lastSeen": nullableText,
		}, "metricRef", "name", "metricType", "seriesCount", "seriesCardinality", "dataPointCount", "lastValue", "lastSeen")),
		"SearchQuery": map[string]any{"type": []string{"object", "null"}, "description": "null selects all. A condition is {id,type:'condition',query:{field:{name,searchScope,type,attributeScope?},fieldOperator,value}}. Groups combine conditions; field/operator/type combinations are validated by the signal store. See /llms.txt for search examples."},
		"Sort":        map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "object", "required": []string{"field", "direction"}, "additionalProperties": false, "properties": map[string]any{"field": text, "direction": map[string]any{"enum": []string{"asc", "desc"}}}}}},
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
		"Metric":       rpcObject(map[string]any{"metricRef": text, "name": text, "description": text, "unit": text, "metricType": text, "metadata": attributes, "resource": object, "scope": object, "series": rpcArray(object)}, "metricRef", "name", "description", "unit", "metricType", "metadata", "resource", "scope", "series"),
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
		"ObjectList": rpcArray(object), "StringList": rpcArray(text),
		"DeleteResult": rpcObject(map[string]any{"message": text, "count": integer}, "message", "count"),
		"ImportConfig": rpcObject(map[string]any{"otlpHttpPort": integer}, "otlpHttpPort"),
		"Object":       object, "Integer": integer, "String": text,
	}

	specs := []struct {
		name, result, description string
		required                  int
	}{
		{"rpc.discover", "", "Returns the same OpenRPC document as GET /openrpc.json. No store access is needed.", 0},
		{"getImportConfig", "ImportConfig", "Returns the configured OTLP HTTP import port for the UI. Fails with -32603 when HTTP import is not configured.", 0},
		{"query", "QueryResult", "Runs one read-only SQL SELECT or supported introspection statement, including SHOW TABLES and DESCRIBE. Defaults to 25 rows; limit 0 returns no rows. A look-ahead row determines truncated. Rows are arrays aligned with columns; SQL NULL is JSON null. Native integer/decimal JSON tokens and JSON columns retain their exact text, not decimal-string conversion: use a precision-preserving parser or SQL VARCHAR casts. Top-level FLOAT/DOUBLE uses finite numbers or OTLP non-finite strings.", 1},
		{"searchTraceSummaries", "TraceSummaries", "Returns trace summaries, not complete spans. Time/query select matching spans; summary counts and bounds describe each whole retained trace. durationNs = max(end_time)-min(start_time), exact decimal nanoseconds. matchedSpans identifies matches. No truncation flag: request a look-ahead row if needed. Omitted/null limit is unbounded.", 2},
		{"getTraceView", "TraceView", "Returns a trace with hierarchy and search-match information. resources/scopes are maps indexed by spanData.r/s database-local sequence keys. traceStart = minimum span start; spanData.start = received start-traceStart and dur = received end-start, exact decimal nanoseconds. depth/matched and kind/status labels are computed display fields; kindCode/statusCodeValue are received signed codes. Each link.summary.target contains name/serviceName derived at read time, or null unless the exact target traceID/spanID pair is stored. Summary metadata is not received OTLP and is excluded from OTLP export. Optional query marks matches without changing received data. Replace each response as a unit, since offsets depend on its baseline.", 1},
		{"getTraceOverview", "TraceOverview", "Returns compact spans and trace-linked log previews. trace.startTime = minimum received span start; durationNs = maximum end-minimum start. startOffsetNs = span start-trace start; span durationNs = end-start. All are exact decimal nanoseconds. spanCount/logCount count retained records. logs.body is a display preview, not the received typed body; severity uses received text or a computed number-range label. Use getSpan/getLog for full data.", 1},
		{"getSpan", "SpanResult", "Looks up a received 16-hex span ID, optionally restricted to a trace. status=found has traceID, span and logs; status=notFound has spanID and nullable traceID; status=ambiguous has spanID, matchCount, summaries and truncated. Without traceID, repeated span IDs across traces are ambiguous, never silently selected. limit defaults to 25 and bounds ambiguity summaries only. Full span startTime/endTime use exact decimal Unix nanoseconds; associated logs preserve typed bodies. Each link.summary.target contains name/serviceName derived at read time, or null unless the exact target traceID/spanID pair is stored. Summary metadata is not received OTLP and is excluded from OTLP export.", 1},
		{"searchLogSummaries", "LogSummaries", "Returns logRef, timestamp, severityText/Number, serviceName and bodyPreview. timestamp is received timestamp unless zero, then observedTimestamp; exact unsigned decimal Unix nanoseconds. bodyPreview is computed display text, not the complete typed body. No truncation flag; omitted/null limit is unbounded.", 2},
		{"getTraceLogSummaries", "LogSummaries", "Returns log summaries carrying the received trace ID, including eventName and nullable spanID. Effective timestamp uses observedTimestamp when received timestamp is zero. bodyPreview is display text.", 1},
		{"getLog", "Log", "Returns one complete normalized log. logRef is a generated database-local ID, not an OTel log ID. timestamp/observedTimestamp are separate received uint64 Unix nanoseconds as decimal strings. Absent trace/span IDs are null. body and attributes retain recursive tagged values. Resource/Scope and schema URLs remain associated with this log.", 1},
		{"searchMetricSummaries", "MetricSummaries", "Returns Metric summaries, not exact datapoints. seriesCount/dataPointCount are window-scoped computed counts; seriesCardinality covers retained datapoints. lastSeen is the maximum window-scoped datapoint timestamp, decimal Unix nanoseconds. lastValue selects the latest scalar value, a computed numeric display projection that may coerce int64 to double. aggregationTemporalityCode is received; its label is computed. Use getMetric/getMetricSeries for exact values. Omitted/null limit is unbounded; no truncation flag.", 2},
		{"getMetric", "Metric", "Returns received Metric identity and a series catalogue. metricRef/seriesRef are generated database-local IDs. Metadata and attributes are typed. Catalogue datapointCount is exact decimal text; first/lastDatapointTimestamp are min/max received timestamps, nullable when no datapoints. Temporality/isMonotonic fields appear only for applicable kinds; unknown received signed temporality codes remain intact.", 1},
		{"getMetricSeries", "MetricSeries", "Returns exact retained received datapoints for one database-local Metric/series pair and inclusive timestamp window. timestamps/startTime, signed intValue, unsigned count/bucketCounts/zeroCount use decimal strings. doubleValue uses finite numbers or IEEE-754 bit text for negative zero/non-finite values. Inactive intValue/doubleValue is null; absent histogram sum/min/max are omitted, not zero. Exemplars and typed attributes stay associated with their datapoints. Empty matching window returns datapoints:[].", 4},
		{"getMetricView", "MetricView", "Returns SQL-computed chart data: Metric metadata, series, statistics, sparklines, histogram/time buckets and aggregate data. Not an exact retained datapoint export; chart calculations can use floating-point display values. See docs/metric-resolution.md for bucket/aggregation formulas and precision. Received numeric payload fields retain exact decimal/typed wire encoding. Optional bucket counts control resolution; 0 uses store defaults. seriesRefs narrows series; selectedSeriesRefs controls aggregate selection; datapointSeriesRefs/Limit bounds datapoint payloads. tzOffsetNs is signed nanoseconds; tzName is the timezone name.", 3},
		{"getMetricAggregateView", "MetricAggregateView", "Returns only the SQL-computed cross-series aggregate chart envelope. Shares metric/window/resolution parameters with getMetricView but has no sparkline/datapoint parameters. selectedSeriesRefs chooses aggregate membership. Not received OTel data; see docs/metric-resolution.md for formulas, units and precision.", 3},
		{"clearTraces", "String", "Deletes all retained traces/spans and sweeps unused shared dictionary rows. Destructive operation.", 0},
		{"clearLogs", "String", "Deletes all retained logs and sweeps unused shared dictionary rows. Destructive operation.", 0},
		{"clearMetrics", "String", "Deletes all retained Metrics and sweeps unused shared dictionary rows. Destructive operation.", 0},
		{"deleteMetric", "String", "Deletes one database-local Metric and dependent rows. Destructive operation.", 1},
		{"deleteSpansByTraceID", "DeleteResult", "Deletes spans for one or more received trace IDs. Variadic positional params: [id1,id2,...], NOT [[id1,id2]]. Nonempty named params are rejected. The documented traceID slot repeats for every supplied ID. count is the number of supplied IDs, not deleted spans. Destructive operation.", 1},
		{"deleteLogsByRefs", "DeleteResult", "Deletes logs by database-local references. Variadic positional params: [ref1,ref2,...], NOT [[ref1,ref2]]. Nonempty named params are rejected. The documented logRef slot repeats. count is supplied references, not deleted rows. Destructive operation.", 1},
		{"getTraceAttributeDefinitions", "AttributeDefinitions", "Returns attribute key/type/scope definitions used by trace filters. No time bounds.", 0},
		{"getLogAttributeDefinitions", "AttributeDefinitions", "Returns attribute key/type/scope definitions used by log filters. No time bounds.", 0},
		{"getMetricAttributeDefinitions", "AttributeDefinitions", "Returns attribute key/type/scope definitions used by Metric filters. No time bounds.", 0},
		{"getTraceAttributeDefinitionsByTraceID", "AttributeDefinitions", "Returns attribute definitions for one received trace ID.", 1},
		{"getFieldValueCompletions", "StringList", "Returns distinct text completion values from an allowlisted signal field. signal is traces/logs/metrics. term is a case-insensitive literal substring; empty matches all values. limit is clamped to 1..500; unknown signal returns -32602 and unsupported field returns -32007. Values are display/search text, not typed received attributes.", 4},
		{"searchAttributeMatches", "ObjectList", "Searches the shared attribute dictionary across signals, without time bounds. Returns matching key/type/scope definitions, not telemetry records.", 1},
		{"getStats", "Stats", "Returns computed signal counts, source recency, storage byte size/retention cap and ingestion rejections. Counts are retained records, not request volume. traces.lastReceived = max(start_time); logs.lastReceived = max(nonzero timestamp), falling back to max(observed_timestamp) only when no nonzero timestamp exists; metrics.lastReceived = max(datapoint timestamp). These are nullable decimal Unix nanoseconds, not wall-clock arrival times. maxSizeBytes=0 disables retention.", 0},
		{"getTraceSpanCount", "Integer", "Returns computed retained span count for a received trace ID as a JSON integer token. A trace with no spans has count 0.", 1},
	}
	methods := make([]rpcMethodDescription, 0, len(specs))
	for _, spec := range specs {
		method := rpcMethodDescription{Name: spec.name, Description: spec.description, ParamStructure: "either", Params: []rpcDescriptor{}, Result: rpcDescriptor{Name: "result", Required: true, Schema: rpcSchemaRef(spec.result)}}
		if spec.name == "rpc.discover" {
			method.Result.Schema = map[string]any{"$ref": openRPCMetaSchema}
		} else if spec.name != "getImportConfig" {
			method.Errors = []map[string]any{{"code": ErrCodeRequestCanceled, "message": "Request canceled"}}
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
			case "traceID", "logRef", "metricRef", "seriesRef":
				schema = rpcSchemaRef("EntityID")
				description = "Received trace ID (32 hex or dashed UUID)."
				if name != "traceID" {
					description = "Generated database-local reference from this viewer, not a received OTel identifier."
				}
			case "spanID":
				schema = map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{16}$"}
			case "startTime", "endTime":
				schema = rpcSchemaRef("TimeBound")
				description = "Required positional slot; null means unbounded. Named omission becomes null when a later named argument supplies the positional gap. Trailing omissions can still fail the method's minimum parameter count."
			case "query":
				schema = rpcSchemaRef("SearchQuery")
			case "sort":
				schema = rpcSchemaRef("Sort")
			case "limit":
				schema = rpcSchemaRef("IntInput")
				description = "Positive int64 whole number; omitted/null search limit is unbounded."
				if spec.name == "query" {
					schema = map[string]any{"type": "integer", "minimum": 0, "maximum": json.Number("18446744073709551615"), "default": 25}
					description = "Unsigned uint64 JSON number only, not a string or null; default 25."
				} else if spec.name == "getSpan" {
					description = "Default 25 when omitted/null; 1 through 9223372036854775806. Bounds ambiguity summaries only."
				} else if spec.name == "getFieldValueCompletions" {
					description = "Signed int64 whole number, clamped to 1..500. Required and non-null."
				}
			case "targetBuckets", "viewBuckets", "sparklineBuckets", "datapointSeriesLimit", "tzOffsetNs":
				schema = rpcSchemaRef("IntInput")
				description = "Omitted/null becomes 0. Must be non-negative except tzOffsetNs, which is signed nanoseconds."
			case "seriesRefs", "selectedSeriesRefs", "datapointSeriesRefs":
				schema = rpcArray(text)
				description = "Database-local series references. Omitted/null uses store defaults."
			case "quantiles":
				schema = rpcArray(map[string]any{"type": "number", "minimum": 0, "maximum": 1})
			case "signal":
				schema = map[string]any{"enum": []string{"traces", "logs", "metrics"}}
			case "field":
				schema = map[string]any{"enum": []string{"name", "eventName", "unit"}}
				description = "Allowed pairs: traces/name, logs/eventName, metrics/name or metrics/unit."
			case "sql":
				description = "One read-only SQL statement. Discover tables with SHOW TABLES and columns with DESCRIBE <table>."
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
		}
		if spec.name == "query" || spec.name == "getTraceView" || spec.name == "searchTraceSummaries" || spec.name == "searchLogSummaries" || spec.name == "searchMetricSummaries" || spec.name == "getFieldValueCompletions" {
			method.Errors = append(method.Errors, map[string]any{"code": ErrCodeInvalidQuery, "message": "Invalid query"})
		}
		method.Examples = rpcExamples(spec.name)
		methods = append(methods, method)
	}
	return map[string]any{
		"openrpc": "1.3.2", "$schema": openRPCMetaSchema,
		"info":    map[string]any{"title": "OTel Desktop Viewer JSON-RPC", "version": "1.0.0", "description": "POST JSON-RPC 2.0 requests to /rpc. Named params and positional params share validation; unknown named keys fail. Required flags describe positional slots. Named interior gaps become null, including nullable time bounds; trailing omissions can still fail minimum arity. null time bounds are unbounded, not zero. Schemas describe values; descriptions also explain lexical and store-specific validation. Standard errors: -32700 parse error, -32600 invalid request, -32601 unknown method, -32602 invalid params, -32603 internal error. HTTP RPC responses use application/json and preserve the request id. Methods ending in View return UI-shaped data intended for rendering in the viewer. Wire values are JSON, not devalue; see each method's description before treating display projections as received OTel data."},
		"servers": []map[string]any{{"name": "viewer", "url": "/rpc"}},
		"methods": methods, "components": map[string]any{"schemas": schemas},
	}
}

func rpcExamples(method string) []map[string]any {
	var params []map[string]any
	var result any
	switch method {
	case "query":
		params = []map[string]any{{"name": "sql", "value": "SELECT 9007199254740993::BIGINT AS exact"}}
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
	return []map[string]any{{"name": "empty-store", "description": "Executable example against an empty viewer store. Named params use these names as keys; positional params use the listed values in order.", "params": params, "result": map[string]any{"name": "result", "value": result}}}
}

func openRPCHandler() (http.Handler, error) {
	document, err := json.Marshal(openRPCDocument())
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(document)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", revalidateCacheControl)
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "openrpc.json", time.Time{}, bytes.NewReader(document))
	}), nil
}
