package server

import "net/http"

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
		{"/openapi.json", "httpDescription", "HTTP API description", "application/json"},
		{"/openrpc.json", "rpcDescription", "JSON-RPC methods and value schemas; start here for agent inspection", "application/json"},
		{"/llms.txt", "agentGuide", "Agent inspection guide and CLI/SQL examples", "text/plain"},
	} {
		paths[doc.path] = map[string]any{
			"get": map[string]any{"operationId": doc.id, "summary": doc.summary, "tags": []string{"Discovery"}, "responses": map[string]any{
				"200": apiResponse(doc.summary, doc.media, map[string]any{}),
				"304": map[string]any{"description": "Unchanged document; ETag matched If-None-Match."},
			}},
			"head": map[string]any{"operationId": doc.id + "Headers", "summary": doc.summary + " headers", "tags": []string{"Discovery"}, "responses": map[string]any{
				"200": map[string]any{"description": "Document headers, including Content-Type, Content-Length and ETag; no body."},
				"304": map[string]any{"description": "Unchanged document; no body."},
			}},
		}
	}
	paths["/rpc"] = map[string]any{"post": map[string]any{
		"operationId": "jsonRPC", "summary": "JSON-RPC inspection and control methods", "tags": []string{"JSON-RPC"},
		"description":  "Agents should use the inspection methods described by /openrpc.json and the workflows in /llms.txt. Start with compact signal summaries, then getSpan/getLog/getMetric/getMetricSeries for full detail. Methods ending in View return computed UI projections. The CLI uses this endpoint and renders tables or JSON. Some methods mutate the store; consult OpenRPC before calling them. Requests are limited to 1 MiB; method errors use the JSON-RPC error envelope rather than HTTP error status. Send one request with an id; batch requests are not supported.",
		"externalDocs": map[string]any{"url": "/openrpc.json", "description": "Authoritative method parameters, results and error codes"},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
			"schema": map[string]any{"type": "object", "required": []string{"jsonrpc", "method", "id"}, "properties": map[string]any{
				"jsonrpc": map[string]any{"const": "2.0"}, "method": text,
				"id":     map[string]any{"type": []string{"string", "integer"}},
				"params": map[string]any{"type": []string{"object", "array", "null"}},
			}},
			"example": map[string]any{"jsonrpc": "2.0", "id": 1, "method": "searchMetricSummaries", "params": map[string]any{"startTime": nil, "endTime": nil, "limit": 25}},
		}}},
		"responses": map[string]any{
			"200": apiResponse("JSON-RPC result or error envelope. Exact numeric tokens require a precision-preserving parser.", "application/json", object),
			"500": apiResponse("Response encoding failed.", "text/plain", text),
		},
	}}
	paths["/api/features"] = map[string]any{"get": map[string]any{
		"operationId": "features", "summary": "Build-selected feature availability", "tags": []string{"Discovery"},
		"responses": map[string]any{"200": apiResponse("Current build features; Cache-Control: no-store.", "application/json", map[string]any{
			"type": "object", "required": []string{"evals"}, "properties": map[string]any{"evals": map[string]any{
				"type": "object", "required": []string{"enabled", "label"}, "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}, "label": text},
			}},
		})},
	}}
	arrowSuccess := apiResponse("Native Arrow IPC stream; Cache-Control: no-store. An interrupted stream is incomplete and must be discarded.", arrowStreamContentType, map[string]any{"type": "string", "format": "binary"})
	arrowSuccess["headers"] = map[string]any{"X-Arrow-Rows": map[string]any{"description": "Expected total row count; validate the complete stream.", "schema": map[string]any{"type": "integer", "minimum": 0}}}
	paths["/api/data/{method}"] = map[string]any{"post": map[string]any{
		"operationId": "browserDataset", "summary": "Browser-oriented native Arrow dataset", "tags": []string{"Browser datasets"},
		"description":  "Browser-oriented transport, not an agent JSON endpoint. For agent inspection use /rpc with /openrpc.json and /llms.txt. The body contains parameters directly, without a JSON-RPC envelope, and is limited to 1 MiB. Supported counterparts share OpenRPC parameter names and validation, but return Arrow datasets rather than the documented JSON results. getSpan and getSpanLogs require both spanID and traceID; getSpanLogs uses getSpan parameter order. getTraceColumnPreviews takes traceID and columns. Ordinary getTraceView returns lightweight waterfall data; a bounded spanIDs selection returns complete selected evidence. No arbitrary SQL is accepted.",
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
		"200": apiResponse("Complete selected retained OTLP JSON. Content-Disposition is attachment; Cache-Control is no-store. Preserve bytes for exact numeric representations.", "application/json", object),
		"206": apiResponse("Requested byte range of the OTLP JSON document; not necessarily a standalone JSON value.", "application/json", map[string]any{"type": "string", "format": "binary"}),
		"400": apiResponse("Invalid identifier, selection, duplicate parameter or unsupported format.", "text/plain", text),
		"404": apiResponse("Signal or selected record not found.", "text/plain", text),
		"416": apiResponse("Requested byte range cannot be satisfied.", "text/plain", text),
		"422": apiResponse("Unsupported stored metric type.", "text/plain", text),
		"500": apiResponse("Export reconstruction failed.", "text/plain", text),
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
		"get":  map[string]any{"operationId": "exportOTLP", "summary": "Download retained telemetry as OTLP JSON", "tags": []string{"Export"}, "description": "Reconstructs surviving telemetry and its received context, not original request bytes. No time or search filter is implied. Trace exports do not include correlated logs. Unknown or signal-inapplicable query parameters are rejected.", "responses": exportResponses},
		"head": map[string]any{"operationId": "exportOTLPHeaders", "summary": "Inspect export headers and byte length", "tags": []string{"Export"}, "responses": map[string]any{"200": map[string]any{"description": "Same export validation and reconstruction as GET, but no response body. Content-Length reports the document size."}, "default": map[string]any{"description": "Same status codes as GET, without a response body."}}},
	}
	addEvalsOpenAPI(paths)
	return map[string]any{
		"openapi":      "3.1.0",
		"info":         map[string]any{"title": "OTel Desktop Viewer HTTP API", "version": "1.0.0", "description": "HTTP routes on the viewer's browser port. Agents: start with /llms.txt and /openrpc.json, then use JSON-RPC inspection methods or the table/JSON CLI. Arrow datasets are browser-oriented. OpenRPC remains authoritative for JSON-RPC method schemas. OTLP receiver endpoints use their separately configured ports and the OpenTelemetry protocol."},
		"servers":      []any{map[string]any{"url": "/"}},
		"externalDocs": map[string]any{"url": "/llms.txt", "description": "Agent inspection guide"},
		"tags":         []any{map[string]any{"name": "Discovery"}, map[string]any{"name": "JSON-RPC"}, map[string]any{"name": "Browser datasets"}, map[string]any{"name": "Export"}},
		"paths":        paths,
	}
}

func apiResponse(description, media string, schema map[string]any) map[string]any {
	return map[string]any{"description": description, "content": map[string]any{media: map[string]any{"schema": schema}}}
}

func apiErrorSchema() map[string]any {
	return map[string]any{"type": "object", "required": []string{"code", "message"}, "properties": map[string]any{"code": map[string]any{"type": "integer"}, "message": map[string]any{"type": "string"}}}
}

func openAPIHandler() (http.Handler, error) {
	return apiDocumentHandler("openapi.json", openAPIDocument())
}
