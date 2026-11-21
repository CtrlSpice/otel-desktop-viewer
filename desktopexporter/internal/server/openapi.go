package server

import (
	"net/http"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
)

// OpenAPI describes HTTP transport. OpenRPC remains the method/schema authority
// for JSON-RPC; Arrow record schemas belong to their datasets, not JSON schemas.
func openAPIDocument() map[string]any {
	text := map[string]any{"type": "string"}
	object := map[string]any{"type": "object"}
	parameter := func(name, location, description string, schema map[string]any) map[string]any {
		return map[string]any{"name": name, "in": location, "required": location == "path", "description": description, "schema": schema}
	}
	paths := map[string]any{}
	for _, doc := range []struct{ path, id, summary, media string }{
		{"/openapi.json", "httpDescription", "HTTP route reference", "application/json"},
		{"/openapi/{operationId}.json", "httpOperation", "One HTTP operation with its parameters and responses", "application/json"},
		{"/openrpc.json", "rpcDescription", "Full JSON-RPC method reference", "application/json"},
		{"/openrpc/index.json", "rpcMethodIndex", "List JSON-RPC method names and summaries", "application/json"},
		{"/openrpc/{method}.json", "rpcMethodDescription", "Single method reference with schemas", "application/json"},
		{"/llms.txt", "agentGuide", "CLI guide and SQL example", "text/plain"},
	} {
		schema := map[string]any{}
		if doc.id == "rpcMethodIndex" {
			schema = rpcObject(map[string]any{"methods": rpcArray(rpcObject(map[string]any{"name": text, "summary": text}, "name", "summary"))}, "methods")
		}
		paths[doc.path] = map[string]any{
			"get": map[string]any{"operationId": doc.id, "summary": doc.summary, "tags": []string{"Discovery"}, "responses": map[string]any{
				"200": apiResponse(doc.summary, doc.media, schema),
				"304": map[string]any{"description": "ETag matches If-None-Match."},
			}},
			"head": map[string]any{"operationId": doc.id + "Headers", "summary": doc.summary + " headers", "tags": []string{"Discovery"}, "responses": map[string]any{
				"200": map[string]any{"description": "Content-Type, Content-Length and ETag."},
				"304": map[string]any{"description": "ETag matches If-None-Match."},
			}},
		}
	}
	for _, ref := range []struct{ path, param, description string }{
		{"/openrpc/{method}.json", "method", "Exact JSON-RPC method name, for example getLog."},
		{"/openapi/{operationId}.json", "operationId", "Exact HTTP operation ID, for example jsonRPC."},
	} {
		item := paths[ref.path].(map[string]any)
		item["parameters"] = []any{parameter(ref.param, "path", ref.description, text)}
		for _, verb := range []string{"get", "head"} {
			item[verb].(map[string]any)["responses"].(map[string]any)["404"] = map[string]any{"description": "Unknown operation or method."}
		}
	}
	paths["/rpc"] = map[string]any{"post": map[string]any{
		"operationId": "jsonRPC", "summary": "Call a JSON-RPC method", "tags": []string{"JSON-RPC"},
		"description":  "Send one JSON-RPC 2.0 request with an id (1 MiB maximum). Batches are unsupported. See /openrpc/{method}.json for the method’s parameters, result and errors.",
		"externalDocs": map[string]any{"url": "/openrpc.json", "description": "Full method reference"},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
			"schema": map[string]any{"type": "object", "required": []string{"jsonrpc", "method", "id"}, "properties": map[string]any{
				"jsonrpc": map[string]any{"const": "2.0"}, "method": text,
				"id":     map[string]any{"type": []string{"string", "integer"}},
				"params": map[string]any{"type": []string{"object", "array", "null"}},
			}},
			"example": map[string]any{"jsonrpc": "2.0", "id": 1, "method": "searchMetricSummaries", "params": map[string]any{"startTime": nil, "endTime": nil, "limit": 25}},
		}}},
		"responses": map[string]any{
			"200": apiResponse("JSON-RPC result or method error. Preserve numeric precision when parsing.", "application/json", object),
			"500": apiResponse("Response encoding failed.", "text/plain", text),
		},
	}}
	paths["/api/features"] = map[string]any{"get": map[string]any{
		"operationId": "features", "summary": "Check whether this build includes Evals", "tags": []string{"Discovery"},
		"responses": map[string]any{"200": apiResponse("Current build features; Cache-Control: no-store.", "application/json", map[string]any{
			"type": "object", "required": []string{"evals"}, "properties": map[string]any{"evals": map[string]any{
				"type": "object", "required": []string{"enabled", "label"}, "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}, "label": text},
			}},
		})},
	}}
	paths["/api/changes"] = map[string]any{"get": map[string]any{
		"operationId": "committedChanges", "summary": "Watch for database changes that require a refresh", "tags": []string{"Viewer UI"},
		"description": "Each connection starts with the current process-local revision (decimal). Rerun queries after reconnecting. One event can cover several writes; it cannot identify an import or confirm completion.",
		"responses":   map[string]any{"200": apiResponse("SSE snapshots and keepalive comments until disconnect or server shutdown.", "text/event-stream", text)},
	}}
	paths["/api/import-target/{signal}"] = map[string]any{"post": map[string]any{
		"operationId": "findImportTarget", "summary": "Find a stored record to open after import", "tags": []string{"Viewer UI"},
		"description": "Look up one span, log or metric with its resource and scope (20 MiB maximum). Lookup applies ingestion redaction to logs and metrics. An identical older log can match. On null, retry after a change event. A match does not confirm the whole import.",
		"parameters":  []any{parameter("signal", "path", "Signal to look up.", map[string]any{"type": "string", "enum": []string{"traces", "logs", "metrics"}})},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": object}}},
		"responses": map[string]any{
			"200": apiResponse("Stored record ID, or null if no match exists.", "application/json", map[string]any{"type": []string{"object", "null"}, "required": []string{"id"}, "properties": map[string]any{"id": text, "spanID": text}}),
			"400": apiResponse("Invalid target or unsupported signal.", "text/plain", text),
			"413": apiResponse("Target exceeds 20 MiB.", "text/plain", text),
			"500": apiResponse("Lookup failed.", "text/plain", text),
		},
	}}
	arrowSuccess := apiResponse("Arrow IPC stream; Cache-Control: no-store. Discard interrupted streams.", arrowStreamContentType, map[string]any{"type": "string", "format": "binary"})
	arrowSuccess["headers"] = map[string]any{"X-Arrow-Rows": map[string]any{"description": "Expected total row count; validate the complete stream.", "schema": map[string]any{"type": "integer", "minimum": 0}}}
	paths["/api/data/{method}"] = map[string]any{"post": map[string]any{
		"operationId": "browserDataset", "summary": "Fetch an Arrow dataset for a table or chart", "tags": []string{"Viewer UI"},
		"description":  "Send parameters directly (1 MiB maximum). Shared methods use OpenRPC parameter names and validation. getSpan and getSpanLogs require both IDs in getSpan order. getTraceColumnPreviews takes traceID and columns. getTraceView returns waterfall data by default; pass a bounded spanIDs list for full details of selected spans. Use /rpc for SQL.",
		"externalDocs": map[string]any{"url": "/llms.txt", "description": "Use JSON-RPC or the CLI for agent inspection"},
		"parameters": []any{parameter("method", "path", "Supported native dataset read.", map[string]any{"type": "string", "enum": []string{
			"searchAttributeMatches", "getTraceAttributeDefinitions", "getLogAttributeDefinitions", "getMetricAttributeDefinitions", "getTraceAttributeDefinitionsByTraceID",
			"getFieldValueCompletions", "discoverTelemetry", "getMetricView", "getMetricAggregateView", "getMetric", "getMetricSeriesOrder", "getMetricSeries",
			"getTraceView", "getSpan", "getTraceColumnPreviews", "searchLogPage", "getLog", "getTraceLogSummaries", "getSpanLogs",
			"searchTraceSummaries", "searchMetricSummaries", "searchLogSummaries",
		}})},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": []string{"object", "array", "null"}}}}},
		"responses": map[string]any{
			"200": arrowSuccess,
			"400": apiResponse("Invalid JSON, parameters, query or cancelled request.", "application/json", apiErrorSchema()),
			"404": apiResponse("Unsupported method, missing entity or expired exact-value reference.", "application/json", apiErrorSchema()),
			"413": apiResponse("Parameter body exceeds 1 MiB.", "text/plain", text),
			"500": apiResponse("Internal error before streaming begins.", "application/json", apiErrorSchema()),
		},
	}}
	exportResponses := map[string]any{
		"200": apiResponse("OTLP JSON attachment; Cache-Control: no-store. Preserve the bytes to retain exact numeric values.", "application/json", object),
		"206": apiResponse("Requested byte range of the OTLP JSON document; not necessarily a standalone JSON value.", "application/json", map[string]any{"type": "string", "format": "binary"}),
		"400": apiResponse("Invalid identifier, selection, duplicate parameter or unsupported format.", "text/plain", text),
		"404": apiResponse("Signal or selected record not found.", "text/plain", text),
		"416": apiResponse("Requested byte range cannot be satisfied.", "text/plain", text),
		"422": apiResponse("Unsupported stored metric type.", "text/plain", text),
		"500": apiResponse("Export reconstruction failed.", "text/plain", text),
	}
	exportHeaders := map[string]any{
		"200": map[string]any{"description": "Content-Length gives the export size in bytes."},
		"206": map[string]any{"description": "Headers for the requested byte range."},
	}
	for code, response := range exportResponses {
		if _, ok := exportHeaders[code]; !ok {
			exportHeaders[code] = map[string]any{"description": response.(map[string]any)["description"]}
		}
	}
	paths["/export/{signal}/{id}"] = map[string]any{
		"parameters": []any{
			parameter("signal", "path", "Signal to export.", map[string]any{"type": "string", "enum": []string{"traces", "logs", "metrics"}}),
			parameter("id", "path", "Received trace ID, or viewer-local log/metric reference; hexadecimal UUID with or without dashes.", text),
			parameter("format", "query", "Optional; json is the only supported format.", map[string]any{"type": "string", "enum": []string{"json"}}),
			parameter("spanId", "query", "Traces only: restrict to this 16-hex-digit span ID within the trace.", map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{16}$"}),
			parameter("seriesRef", "query", "Metrics only: select one viewer-local series UUID.", text),
			parameter("datapointRef", "query", "Metrics only: select one viewer-local datapoint UUID.", text),
			parameter("includeDatapoints", "query", "Metrics only: defaults to true; false exports descriptors without datapoints.", map[string]any{"type": "boolean", "default": true}),
			parameter("Range", "header", "Optional HTTP byte range for a bounded preview; omit for complete export.", text),
		},
		"get":  map[string]any{"operationId": "exportOTLP", "summary": "Download retained telemetry as OTLP JSON", "tags": []string{"Export"}, "description": "Rebuilds the payload from stored telemetry and its context. Search and time filters do not apply. Export trace-linked logs separately. Unknown or inapplicable parameters return 400.", "responses": exportResponses},
		"head": map[string]any{"operationId": "exportOTLPHeaders", "summary": "Check the export size without downloading the payload", "tags": []string{"Export"}, "responses": exportHeaders},
	}
	addEvalsOpenAPI(paths)
	tags := []map[string]string{{"name": "Discovery"}, {"name": "JSON-RPC"}, {"name": "Export"}}
	if features.EvalsBuilt {
		tags = append(tags, map[string]string{"name": "Evaluations", "description": "Available in Evals builds."})
	}
	tags = append(tags, map[string]string{"name": "Viewer UI", "description": "These routes serve viewer screens. Use /rpc to inspect records."})
	return map[string]any{
		"openapi":      "3.1.0",
		"info":         map[string]any{"title": "OTel Desktop Viewer HTTP API", "version": "1.0.0", "description": "HTTP routes for querying and exporting stored telemetry and loading data for viewer screens."},
		"servers":      []any{map[string]any{"url": "/"}},
		"externalDocs": map[string]any{"url": "/llms.txt", "description": "Agent inspection guide"},
		"tags":         tags,
		"paths":        paths,
	}
}

func apiResponse(description, media string, schema map[string]any) map[string]any {
	return map[string]any{"description": description, "content": map[string]any{media: map[string]any{"schema": schema}}}
}

func apiErrorSchema() map[string]any {
	return map[string]any{"type": "object", "required": []string{"code", "message"}, "properties": map[string]any{"code": map[string]any{"type": "integer"}, "message": map[string]any{"type": "string"}}}
}

func registerOpenAPI(mux *http.ServeMux) error {
	doc := openAPIDocument()
	full, err := apiDocumentHandler("openapi.json", doc)
	if err != nil {
		return err
	}
	mux.Handle("GET /openapi.json", full)
	for path, value := range doc["paths"].(map[string]any) {
		item := value.(map[string]any)
		for verb, value := range item {
			if verb == "parameters" {
				continue
			}
			op := value.(map[string]any)
			single := map[string]any{verb: op}
			if params, ok := item["parameters"]; ok {
				single["parameters"] = params
			}
			id := op["operationId"].(string)
			url := "/openapi/" + id + ".json"
			handler, err := apiDocumentHandler(id+".json", map[string]any{
				"openapi": doc["openapi"], "info": doc["info"], "servers": doc["servers"],
				"paths": map[string]any{path: single},
			})
			if err != nil {
				return err
			}
			mux.Handle("GET "+url, handler)
		}
	}
	return nil
}
