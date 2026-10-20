package server

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

func TestOpenRPCHTTPAndRPCDiscovery(t *testing.T) {
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	// Discovery must work without a database and must not touch a manual viewer.
	server := httptest.NewServer(s.server.Handler)
	defer server.Close()
	var document []byte
	var etag string
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req, err := http.NewRequest(method, server.URL+"/openrpc.json", nil)
		require.NoError(t, err)
		res, err := server.Client().Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
		if method == http.MethodGet {
			document = body
			etag = res.Header.Get("ETag")
			require.NotEmpty(t, etag)
		} else {
			assert.Empty(t, body)
			assert.Equal(t, int64(len(document)), res.ContentLength)
		}
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/openrpc.json", nil)
	require.NoError(t, err)
	req.Header.Set("If-None-Match", etag)
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	conditionalBody, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, res.StatusCode)
	assert.Empty(t, conditionalBody)
	for _, params := range []string{"", `,"params":[]`, `,"params":{}`, `,"params":null`} {
		res, err := server.Client().Post(server.URL+"/rpc", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":"discovery","method":"rpc.discover"`+params+`}`))
		require.NoError(t, err)
		var response struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
		res.Body.Close()
		assert.Empty(t, response.Error)
		assert.Equal(t, "discovery", response.ID)
		assert.JSONEq(t, string(document), string(response.Result))
	}
	for _, params := range []string{`[1]`, `{"unknown":1}`, `"bad"`} {
		res, err := server.Client().Post(server.URL+"/rpc", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"rpc.discover","params":`+params+`}`))
		require.NoError(t, err)
		var response struct {
			ID    int `json:"id"`
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
		res.Body.Close()
		assert.Equal(t, 7, response.ID)
		assert.Equal(t, -32602, response.Error.Code)
	}
	res, err = server.Client().Get(server.URL + "/")
	require.NoError(t, err)
	index, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	assert.Contains(t, res.Header.Get("Link"), `</openrpc.json>; rel="service-desc"`)
	assert.Contains(t, string(index), `rel="service-desc" type="application/json" href="/openrpc.json"`)
	res, err = server.Client().Get(server.URL + "/llms.txt")
	require.NoError(t, err)
	guide, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	assert.Contains(t, string(guide), "GET /openrpc.json")
	assert.Contains(t, string(guide), "rpc.discover")
}

func TestOpenRPCMethodsParametersAndRequirednessMatchHandlers(t *testing.T) {
	document := openRPCDocument()
	methods := document["methods"].([]rpcMethodDescription)
	names := make([]string, 0, len(methods))
	handler := setupHandler(t)
	handler.features = enabledEvalFlags(t)
	for _, method := range methods {
		names = append(names, method.Name)
		t.Run(method.Name, func(t *testing.T) {
			var documented []string
			var required []any
			optional := false
			for _, param := range method.Params {
				documented = append(documented, param.Name)
				if !param.Required {
					optional = true
				} else {
					require.False(t, optional, "required params must precede optional params")
					switch param.Name {
					case "traceID", "logRef", "metricRef", "seriesRef":
						required = append(required, "00000000000000000000000000000001")
					case "spanID":
						required = append(required, "0000000000000001")
					default:
						required = append(required, nil)
					}
				}
			}
			if method.ParamStructure == "either" {
				assert.Equal(t, methodParamNames[method.Name], documented)
			} else {
				assert.Contains(t, []string{"deleteSpansByTraceID", "deleteLogsByRefs"}, method.Name)
				assert.Contains(t, method.Description, "Variadic positional")
			}
			if len(required) > 0 {
				_, err := handler.Handle(context.Background(), createRequest(method.Name, required[:len(required)-1]))
				assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams, "missing required positional slot must fail before reading the store")
			}
			assert.NotEmpty(t, method.Description)
			assert.NotEmpty(t, method.Result.Schema)
		})
	}
	assert.ElementsMatch(t, dispatchedMethods(), names)
	assert.Len(t, names, len(mapMethodNames(methods)))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"tags"`)
	assert.Equal(t, 1, strings.Count(string(encoded), "Methods ending in View return UI-shaped data intended for rendering in the viewer."))
}

func mapMethodNames(methods []rpcMethodDescription) map[string]bool {
	names := make(map[string]bool, len(methods))
	for _, method := range methods {
		names[method.Name] = true
	}
	return names
}

func TestOpenRPCExamplesExecuteExactly(t *testing.T) {
	handler := setupHandler(t)
	for _, method := range openRPCDocument()["methods"].([]rpcMethodDescription) {
		for _, example := range method.Examples {
			t.Run(method.Name, func(t *testing.T) {
				named := map[string]any{}
				positional := []any{}
				for _, param := range example["params"].([]map[string]any) {
					name := param["name"].(string)
					assert.Contains(t, methodParamNames[method.Name], name)
					named[name] = param["value"]
					positional = append(positional, param["value"])
				}
				want, err := json.Marshal(example["result"].(map[string]any)["value"])
				require.NoError(t, err)
				for _, params := range []any{named, positional} {
					got, err := handler.Handle(context.Background(), createRequest(method.Name, params))
					require.NoError(t, err)
					encoded, err := json.Marshal(got)
					require.NoError(t, err)
					var value any
					require.NoError(t, decodeParams(encoded, &value))
					encoded, err = json.Marshal(value)
					require.NoError(t, err)
					// Canonical key order with UseNumber still catches integer precision loss.
					assert.Equal(t, string(want), string(encoded))
				}
			})
		}
	}
}

func TestOpenRPCSchemasMatchPopulatedWireResults(t *testing.T) {
	handler := setupHandlerWithData(t)
	handler.otlpHTTPPort = 4318
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(context.Background(), conn, buildTestMetrics(), handler.store.FlushedIDs())
	}))
	result, err := handler.Handle(context.Background(), createRequest("query", []any{`SELECT m.id::varchar, s.id::varchar, l.id::varchar FROM metrics m JOIN metric_series s ON s.metric_id = m.id CROSS JOIN logs l LIMIT 1`}))
	require.NoError(t, err)
	var refs struct {
		Rows [][]string `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(result.(json.RawMessage), &refs))
	require.Len(t, refs.Rows, 1)
	require.Len(t, refs.Rows[0], 3)
	metricRef, seriesRef, logRef := refs.Rows[0][0], refs.Rows[0][1], refs.Rows[0][2]
	document := openRPCDocument()
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	for _, tc := range []struct {
		method string
		params any
	}{
		{"getTraceView", []any{testTraceIDHex}},
		{"getTraceOverview", []any{testTraceIDHex}},
		{"getSpan", []any{"0000000000000001", testTraceIDHex}},
		{"query", []any{"SELECT 9007199254740993::BIGINT AS exact"}},
		{"searchTraceSummaries", []any{nil, nil}},
		{"searchLogSummaries", []any{nil, nil}},
		{"searchMetricSummaries", []any{nil, nil}},
		{"getTraceLogSummaries", []any{testTraceIDHex}},
		{"getLog", []any{logRef}},
		{"getMetric", []any{metricRef}},
		{"getMetricSeries", []any{metricRef, seriesRef, nil, nil}},
		{"getMetricView", []any{metricRef, nil, nil}},
		{"getMetricAggregateView", []any{metricRef, nil, nil}},
		{"getTraceAttributeDefinitions", []any{}},
		{"getLogAttributeDefinitions", []any{}},
		{"getMetricAttributeDefinitions", []any{}},
		{"getTraceAttributeDefinitionsByTraceID", []any{testTraceIDHex}},
		{"getFieldValueCompletions", []any{"traces", "name", "", 10}},
		{"searchAttributeMatches", []any{"pumpkin"}},
		{"getStats", []any{}},
		{"getTraceSpanCount", []any{testTraceIDHex}},
		{"getImportConfig", []any{}},
		// Mutations run last against this test-owned store only.
		{"deleteMetric", []any{metricRef}},
		{"deleteSpansByTraceID", []any{testTraceIDHex}},
		{"deleteLogsByRefs", []any{logRef}},
		{"clearTraces", []any{}},
		{"clearLogs", []any{}},
		{"clearMetrics", []any{}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(tc.method, tc.params))
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var value any
			require.NoError(t, decodeParams(encoded, &value))
			for _, method := range document["methods"].([]rpcMethodDescription) {
				if method.Name == tc.method {
					require.NoError(t, checkRPCWireSchema(method.Result.Schema, value, schemas))
				}
			}
		})
	}
}

// Checks the schema keywords used for wire-shape assertions; not a general JSON Schema validator.
func checkRPCWireSchema(schema map[string]any, value any, schemas map[string]any) error {
	if ref, ok := schema["$ref"].(string); ok {
		return checkRPCWireSchema(schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any), value, schemas)
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if alternatives, ok := schema[keyword].([]any); ok {
			matches := 0
			for _, alternative := range alternatives {
				if checkRPCWireSchema(alternative.(map[string]any), value, schemas) == nil {
					matches++
				}
			}
			if matches == 0 || keyword == "oneOf" && matches != 1 {
				return errors.New("schema alternative mismatch")
			}
		}
	}
	if constant, ok := schema["const"]; ok && constant != value {
		return errors.New("schema constant mismatch")
	}
	if enum, ok := schema["enum"].([]string); ok {
		found := false
		for _, candidate := range enum {
			if candidate == value {
				found = true
			}
		}
		if !found {
			return errors.New("schema enum mismatch")
		}
	}
	var types []string
	switch typ := schema["type"].(type) {
	case string:
		types = []string{typ}
	case []string:
		types = typ
	}
	if len(types) > 0 {
		matched := false
		for _, typ := range types {
			switch typ {
			case "null":
				matched = matched || value == nil
			case "string":
				_, ok := value.(string)
				matched = matched || ok
			case "boolean":
				_, ok := value.(bool)
				matched = matched || ok
			case "object":
				_, ok := value.(map[string]any)
				matched = matched || ok
			case "array":
				_, ok := value.([]any)
				matched = matched || ok
			case "number", "integer":
				n, ok := value.(json.Number)
				if typ == "integer" {
					_, err := n.Int64()
					ok = ok && err == nil
				}
				matched = matched || ok
			}
		}
		if !matched {
			return errors.New("schema type mismatch")
		}
	}
	if pattern, ok := schema["pattern"].(string); ok {
		text, ok := value.(string)
		if !ok || !regexp.MustCompile(pattern).MatchString(text) {
			return errors.New("schema pattern mismatch")
		}
	}
	if object, ok := value.(map[string]any); ok {
		if required, ok := schema["required"].([]string); ok {
			for _, key := range required {
				if _, ok := object[key]; !ok {
					return errors.New("missing required wire field: " + key)
				}
			}
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			for key, child := range properties {
				if v, ok := object[key]; ok {
					if err := checkRPCWireSchema(child.(map[string]any), v, schemas); err != nil {
						return errors.New(key + ": " + err.Error())
					}
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		if items, ok := schema["items"].(map[string]any); ok {
			for _, item := range array {
				if err := checkRPCWireSchema(items, item, schemas); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestOpenRPCExactTimeInputs(t *testing.T) {
	for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
		for _, raw := range []string{`{"startTime":"18446744073709551615","endTime":null}`, `{"startTime":18446744073709551615,"endTime":null}`} {
			normalized, err := normalizeParams(method, json.RawMessage(raw))
			require.NoError(t, err)
			params, err := parseSearchParams(normalized)
			require.NoError(t, err)
			require.NotNil(t, params.timeRange.Start)
			assert.Equal(t, ^uint64(0), *params.timeRange.Start)
			assert.Nil(t, params.timeRange.End)
		}
		normalized, err := normalizeParams(method, json.RawMessage(`{"limit":1}`))
		require.NoError(t, err)
		params, err := parseSearchParams(normalized)
		require.NoError(t, err)
		assert.Nil(t, params.timeRange.Start)
		assert.Nil(t, params.timeRange.End)
	}
	// Confirm serialization does not round the schema's numeric maximum either.
	encoded, err := json.Marshal(openRPCDocument())
	require.NoError(t, err)
	assert.True(t, bytes.Contains(encoded, []byte(`"maximum":18446744073709551615`)))
}
