//go:build evals

package server

func addEvalsOpenAPI(paths map[string]any) {
	text := map[string]any{"type": "string"}
	object := map[string]any{"type": "object"}
	param := func(name, description string, schema map[string]any) map[string]any {
		return map[string]any{"name": name, "in": "query", "description": description, "schema": schema}
	}
	paths["/api/evals/ingest"] = map[string]any{"post": map[string]any{
		"operationId": "ingestEvals", "summary": "Import a Promptfoo evaluation file", "tags": []string{"Evaluations"},
		"description": "Upload raw source bytes: a full Promptfoo v3 envelope, or JSONL of complete independent envelopes; bare result rows are rejected. Do not wrap in JSON-RPC. Validated records pass through the configured OTLP receiver, redaction and batching. Success confirms retained evidence, not merely receiver acceptance. Cancellation or timeout may leave accepted records that arrive later; retry the same source to reconcile completion.",
		"parameters":  []any{param("name", "Source filename/provenance, at most 256 UTF-8 bytes; defaults to Imported evaluation.", text)},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": object}, "application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}},
		"responses": map[string]any{
			"200": apiResponse("Completed import.", "application/json", map[string]any{"type": "object", "required": []string{"ids", "created", "complete"}, "properties": map[string]any{"ids": map[string]any{"type": "array", "items": text}, "created": map[string]any{"type": "boolean"}, "complete": map[string]any{"const": true}}}),
			"400": apiResponse("Invalid name or source input.", "text/plain", text),
			"409": apiResponse("Identical import already in progress.", "text/plain", text),
			"422": apiResponse("Retained evidence is incomplete or conflicting.", "text/plain", text),
			"500": apiResponse("Staging, persistence lookup or projection failed.", "text/plain", text),
			"502": apiResponse("Receiver submission failed.", "text/plain", text),
			"504": apiResponse("Persistence was not confirmed before timeout/cancellation; accepted telemetry may still arrive.", "text/plain", text),
		},
	}}
	paths["/api/evals/runs/{runID}/results/{resultID}"] = map[string]any{"put": map[string]any{
		"operationId": "putEvalResult", "summary": "Publish one Promptfoo result", "tags": []string{"Evaluations"},
		"description": "Repeating the same request counts once. Changing the body for the same run and result ID returns 409.",
		"parameters": []any{
			map[string]any{"name": "runID", "in": "path", "required": true, "schema": text},
			map[string]any{"name": "resultID", "in": "path", "required": true, "schema": text},
		},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"id", "provider"}, "properties": map[string]any{"id": text, "provider": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": text}}}}}}},
		"responses": map[string]any{
			"200": apiResponse("Result retained.", "application/json", map[string]any{"type": "object", "required": []string{"resultID", "datasetID", "ordinal", "complete", "retainedSHA256"}, "properties": map[string]any{"resultID": text, "datasetID": text, "ordinal": map[string]any{"type": "integer"}, "complete": map[string]any{"const": true}, "retainedSHA256": map[string]any{"type": "string", "description": "SHA-256 of the redacted row JSON returned by the viewer."}}}),
			"400": apiResponse("Invalid result.", "text/plain", text),
			"409": apiResponse("Conflicting result or publication in progress.", "text/plain", text),
			"500": apiResponse("Result storage or verification failed.", "text/plain", text),
			"502": apiResponse("Receiver submission failed.", "text/plain", text),
			"504": apiResponse("Persistence not confirmed.", "text/plain", text),
		},
	}}
	responses := func(description string, schema map[string]any) map[string]any {
		return map[string]any{"200": apiResponse(description, "application/json", schema), "400": apiResponse("Invalid query parameters.", "text/plain", text), "404": apiResponse("Evals unavailable or experiment not found.", "text/plain", text), "500": apiResponse("Read or evidence reconciliation failed.", "text/plain", text)}
	}
	paths["/api/evals/experiments"] = map[string]any{"get": map[string]any{
		"operationId": "searchEvalExperiments", "summary": "Search retained evaluation experiments", "tags": []string{"Evaluations"},
		"description": "Uses the shared search criteria tree; q is not supported. Returns computed experiment summaries and index status, not complete source rows.",
		"parameters": []any{
			param("criteria", "JSON-encoded search tree, at most 64 KiB UTF-8; URL-encode the value.", text),
			param("outcome", "Repeat to select recorded outcome keys, at most five.", map[string]any{"type": "array", "maxItems": 5, "items": text}),
			param("sort", "Sort key; defaults to started.", map[string]any{"type": "string", "enum": []string{"name", "started"}, "default": "started"}),
			param("direction", "Defaults to asc for name and desc for started.", map[string]any{"type": "string", "enum": []string{"asc", "desc"}}),
			param("limit", "Maximum summaries.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}),
			param("offset", "Matching summaries to skip.", map[string]any{"type": "integer", "minimum": 0, "maximum": 1000000, "default": 0}),
		},
		"responses": responses("Experiment page; Cache-Control: no-store.", map[string]any{"type": "object", "required": []string{"experiments", "total", "hasMore", "facets", "indexing"}, "properties": map[string]any{"experiments": map[string]any{"type": "array", "items": object}, "total": map[string]any{"type": "integer"}, "hasMore": map[string]any{"type": "boolean"}, "facets": map[string]any{"type": "array", "items": object}, "indexing": object}}),
	}}
	paths["/api/evals/experiments/{id}/rows"] = map[string]any{"get": map[string]any{
		"operationId": "evalExperimentRows", "summary": "Read one page of experiment rows", "tags": []string{"Evaluations"},
		"description": "contextJSON and each row's json are source JSON strings preserving numeric lexemes; use a precision-preserving parser if decoding them.",
		"parameters": []any{
			map[string]any{"name": "id", "in": "path", "required": true, "schema": text, "description": "Viewer-local experiment ID."},
			param("after", "Exclusive source-row ordinal cursor; -1 starts at the beginning.", map[string]any{"type": "integer", "minimum": -1, "default": -1}),
			param("limit", "Maximum rows.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 100}),
		},
		"responses": responses("Source row page; Cache-Control: no-store.", map[string]any{"type": "object", "required": []string{"summary", "contextJSON", "rows", "nextAfter", "hasMore"}, "properties": map[string]any{
			"summary": object, "contextJSON": text, "nextAfter": map[string]any{"type": "integer"}, "hasMore": map[string]any{"type": "boolean"},
			"rows": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"ordinal", "json"}, "properties": map[string]any{"ordinal": map[string]any{"type": "integer"}, "json": text}}},
		}}),
	}}
}
