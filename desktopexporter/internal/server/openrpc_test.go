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
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server/apidoc"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server/rpc"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

const testOpenRPCTraceID = "00000000000000000000000000000001"

func newRPCTestHandler(t *testing.T) (*rpc.Handler, *store.Store) {
	t.Helper()
	st, _ := storetest.New(t)
	h := rpc.NewHandler(st, zap.NewNop())
	h.SetDiscoverDocument(apidoc.OpenRPCDocument)
	return h, st
}

func newRPCTestHandlerWithData(t *testing.T) (*rpc.Handler, *store.Store) {
	t.Helper()
	h, st := newRPCTestHandler(t)
	base := time.Now().UnixNano()

	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "pumpkin.pie")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID([16]byte{15: 1})
	span.SetSpanID([8]byte{7: 1})
	span.SetName("test")
	span.SetStartTimestamp(pcommon.Timestamp(base))
	span.SetEndTimestamp(pcommon.Timestamp(base + time.Second.Nanoseconds()))
	require.NoError(t, st.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(t.Context(), conn, tr)
	}))

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "pumpkin.pie")
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.SetTimestamp(pcommon.Timestamp(base))
	rec.SetTraceID([16]byte{15: 1})
	rec.SetSpanID([8]byte{7: 1})
	rec.Body().SetStr("test log message")
	rec.SetEventName("request.failed")
	rec.SetSeverityText("INFO")
	rec.SetSeverityNumber(plog.SeverityNumberInfo)
	require.NoError(t, st.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(t.Context(), conn, ld)
	}))

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-svc")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")
	met := sm.Metrics().AppendEmpty()
	met.SetName("test.gauge")
	met.SetDescription("A test gauge")
	met.SetUnit("bytes")
	dp := met.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base))
	dp.SetDoubleValue(42.0)
	require.NoError(t, st.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(t.Context(), conn, md)
	}))

	return h, st
}

func createRPCRequest(method string, params any) *jsonrpc2.Request {
	paramsBytes, _ := json.Marshal(params)
	return &jsonrpc2.Request{
		Method: method,
		Params: paramsBytes,
		ID:     jsonrpc2.Int64ID(1),
	}
}

func dispatchedRPCMethods() []string {
	src, err := os.ReadFile("rpc/handler.go")
	if err != nil {
		panic(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`case "([a-zA-Z.]+)":`).FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return out
}

type stubDatabaseManager struct {
	list DatabaseList
}

func (m *stubDatabaseManager) ListDatabases(context.Context) (DatabaseList, error) {
	return m.list, nil
}

func (m *stubDatabaseManager) OpenDatabase(_ context.Context, name string) (DatabaseList, error) {
	m.list.Active = name
	for i := range m.list.Databases {
		m.list.Databases[i].Active = m.list.Databases[i].Name == name
	}
	return m.list, nil
}

func (m *stubDatabaseManager) NewDatabase(_ context.Context, name string) (DatabaseList, error) {
	m.list.Active = name
	m.list.Databases = append(m.list.Databases, DatabaseEntry{
		Name:       name,
		SizeBytes:  4096,
		ModifiedAt: "2026-11-26T18:00:00Z",
		Active:     true,
	})
	return m.list, nil
}

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
	assert.Contains(t, string(guide), "/openrpc.json")
	assert.Contains(t, string(guide), "/openapi.json")
}

func TestOpenRPCIndividualMethodDocuments(t *testing.T) {
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	read := func(verb, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(verb, path, nil))
		return w
	}
	type document struct {
		Methods    []json.RawMessage                            `json:"methods"`
		Components struct{ Schemas map[string]json.RawMessage } `json:"components"`
	}
	var full document
	require.NoError(t, json.Unmarshal(read("GET", "/openrpc.json").Body.Bytes(), &full))
	w := read("GET", "/openrpc/index.json")
	require.Equal(t, http.StatusOK, w.Code)
	var index struct{ Methods []map[string]string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &index))
	require.Len(t, index.Methods, len(full.Methods))
	for i, method := range full.Methods {
		var entry struct{ Name, Summary string }
		require.NoError(t, json.Unmarshal(method, &entry))
		require.NotEmpty(t, entry.Summary)
		require.Equal(t, map[string]string{"name": entry.Name, "summary": entry.Summary}, index.Methods[i])
		t.Run(entry.Name, func(t *testing.T) {
			path := "/openrpc/" + entry.Name + ".json"
			w := read("GET", path)
			require.Equal(t, http.StatusOK, w.Code)
			var single document
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &single))
			require.Len(t, single.Methods, 1)
			require.Equal(t, method, single.Methods[0])
			for name, schema := range single.Components.Schemas {
				if name == "DiscoveryRows" {
					var link struct {
						Ref string `json:"$ref"`
					}
					require.NoError(t, json.Unmarshal(schema, &link))
					path, target, ok := strings.Cut(link.Ref, "#/components/schemas/")
					require.True(t, ok)
					var linked document
					require.NoError(t, json.Unmarshal(read("GET", path).Body.Bytes(), &linked))
					require.Equal(t, full.Components.Schemas["DiscoveryRows"], linked.Components.Schemas[target])
					continue
				}
				require.Equal(t, full.Components.Schemas[name], schema)
			}
			for _, ref := range regexp.MustCompile(`"#/components/schemas/([^"]+)"`).FindAllStringSubmatch(w.Body.String(), -1) {
				require.Contains(t, single.Components.Schemas, ref[1])
			}
			if entry.Name == "getLog" {
				require.Contains(t, single.Components.Schemas, "Value")
				require.NotContains(t, single.Components.Schemas, "Metric")
				head := read("HEAD", path)
				require.Equal(t, http.StatusOK, head.Code)
				require.Empty(t, head.Body.Bytes())
				require.Equal(t, w.Header().Get("ETag"), head.Header().Get("ETag"))
			}
		})
	}
	require.Equal(t, http.StatusNotFound, read("GET", "/openrpc/unknown.json").Code)
}

func checkDiscoveryRows(t *testing.T, raw json.RawMessage, kind string) {
	t.Helper()
	var result map[string]any
	require.NoError(t, rpc.DecodeParams(raw, &result))
	schemas := apidoc.OpenRPCDocument()["components"].(map[string]any)["schemas"].(map[string]any)
	require.NoError(t, checkRPCWireSchema(apidoc.SchemaRef("DiscoveryRows"), result, schemas))
	rows := schemas["DiscoveryRows"].(map[string]any)["properties"].(map[string]any)["rows"].(map[string]any)
	for _, alternative := range rows["anyOf"].([]any) {
		page := alternative.(map[string]any)
		if page["title"] == kind {
			require.NoError(t, checkRPCWireSchema(page, result["rows"], schemas))
			return
		}
	}
	t.Fatalf("missing result schema for %s", kind)
}

func TestOpenRPCDatabaseAndDiscoveryWireSchemas(t *testing.T) {
	h, _ := newRPCTestHandlerWithData(t)
	h.SetDatabaseManager(&stubDatabaseManager{
		list: DatabaseList{
			Managed: true,
			Active:  "current.duckdb",
			Databases: []DatabaseEntry{
				{Name: "current.duckdb", SizeBytes: 12288, ModifiedAt: "2026-11-26T17:00:00Z", Active: true},
				{Name: "checkout.duckdb", SizeBytes: 24576, ModifiedAt: "2026-11-26T16:00:00Z", Active: false},
			},
		},
	})

	doc := apidoc.OpenRPCDocument()
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	dbListProps := schemas["DatabaseList"].(map[string]any)["properties"].(map[string]any)
	assert.NotContains(t, dbListProps, "directory", "DatabaseList schema must not expose local filesystem paths")

	methodFor := func(name string) apidoc.RPCMethodDescription {
		for _, m := range doc["methods"].([]apidoc.RPCMethodDescription) {
			if m.Name == name {
				return m
			}
		}
		t.Fatalf("missing OpenRPC method %s", name)
		return apidoc.RPCMethodDescription{}
	}
	for _, name := range []string{"openDatabase", "newDatabase"} {
		m := methodFor(name)
		require.Len(t, m.Params, 1)
		assert.Equal(t, "name", m.Params[0].Name)
		assert.NotEmpty(t, m.Params[0].Description)
		assert.Equal(t, 1, m.Params[0].Schema["minLength"])
	}
	for _, tc := range []struct {
		method string
		params any
	}{
		{"listDatabases", []any{}},
		{"openDatabase", map[string]any{"name": "checkout.duckdb"}},
		{"newDatabase", []any{"incident.duckdb"}},
	} {
		got, err := h.Handle(t.Context(), createRPCRequest(tc.method, tc.params))
		require.NoError(t, err)
		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		var decoded any
		require.NoError(t, rpc.DecodeParams(encoded, &decoded))
		require.NoError(t, checkRPCWireSchema(methodFor(tc.method).Result.Schema, decoded, schemas))
	}

	// Verify discovery row schemas across discovery kinds and home-values/home-value.
	servicesRes, err := h.Handle(t.Context(), createRPCRequest("discoverTelemetry", map[string]any{
		"options": map[string]any{"kind": "services"},
	}))
	require.NoError(t, err)
	checkDiscoveryRows(t, servicesRes.(json.RawMessage), "services")

	var servicePage struct {
		Rows [][]struct{ ServiceIdentity string } `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(servicesRes.(json.RawMessage), &servicePage))
	require.NotEmpty(t, servicePage.Rows)
	svcID := servicePage.Rows[0][0].ServiceIdentity

	for _, options := range []map[string]any{
		{"kind": "keys", "signal": "traces", "ownerType": "resource"},
		{"kind": "values", "signal": "traces", "ownerType": "resource", "key": "service.name"},
		{"kind": "connections", "serviceIdentity": svcID},
		{"kind": "connection-spans", "serviceIdentity": svcID, "targetIdentity": svcID},
	} {
		res, err := h.Handle(t.Context(), createRPCRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		checkDiscoveryRows(t, res.(json.RawMessage), options["kind"].(string))
	}

	checkSchema := func(schema map[string]any, data any) {
		t.Helper()
		raw, err := json.Marshal(data)
		require.NoError(t, err)
		var value any
		require.NoError(t, rpc.DecodeParams(raw, &value))
		require.NoError(t, checkRPCWireSchema(schema, value, schemas))
	}
	homeOpts := map[string]any{"kind": "home-values", "ownerType": "resource", "key": "service.name"}
	homeRes, err := h.Handle(t.Context(), createRPCRequest("discoverTelemetry", map[string]any{"options": homeOpts}))
	require.NoError(t, err)
	checkSchema(apidoc.DiscoveryOptionsSchema(), homeOpts)
	checkSchema(apidoc.SchemaRef("DiscoveryResult"), homeRes)

	rawHome, err := json.Marshal(homeRes)
	require.NoError(t, err)
	var homePage struct {
		Values []struct{ ValueRef string } `json:"values"`
	}
	require.NoError(t, json.Unmarshal(rawHome, &homePage))
	require.NotEmpty(t, homePage.Values)
	resolveOpts := map[string]any{"kind": "home-value", "valueRef": homePage.Values[0].ValueRef}
	exactRes, err := h.Handle(t.Context(), createRPCRequest("discoverTelemetry", map[string]any{"options": resolveOpts}))
	require.NoError(t, err)
	checkSchema(apidoc.DiscoveryOptionsSchema(), resolveOpts)
	checkSchema(apidoc.SchemaRef("DiscoveryResult"), exactRes)
}

func TestDiscoverySignalPredicateContract(t *testing.T) {
	h, _ := newRPCTestHandler(t)
	schemas := apidoc.OpenRPCDocument()["components"].(map[string]any)["schemas"].(map[string]any)
	for _, tc := range []struct {
		predicate string
		valid     bool
	}{
		{`{}`, false}, {`{"type":"unknown"}`, false},
		{`{"type":"condition","query":{"fieldOperator":"="}}`, false},
		{`{"type":"condition","query":{"field":null,"fieldOperator":"="}}`, false},
		{`{"type":"condition","query":{"field":{"searchScope":"global"},"fieldOperator":"CONTAINS","value":"test"}}`, true},
		{`{"type":"text","query":{"fieldOperator":"CONTAINS","field":{}}}`, false},
		{`{"type":"group","group":{"logicalOperator":"AND","children":[{}]}}`, false},
		{`null`, true}, {`false`, true},
		{`{"type":"text","query":{"fieldOperator":"CONTAINS","value":"test"}}`, true},
	} {
		t.Run(tc.predicate, func(t *testing.T) {
			var predicate any
			require.NoError(t, rpc.DecodeParams([]byte(tc.predicate), &predicate))
			queries := map[string]any{"traces": predicate}
			options := map[string]any{"kind": "services", "queries": queries}
			_, err := h.Handle(t.Context(), createRPCRequest("discoverTelemetry", map[string]any{"options": options}))
			require.Equal(t, tc.valid, err == nil)
			schema := apidoc.DiscoveryOptionsSchema()["properties"].(map[string]any)["queries"].(map[string]any)["additionalProperties"].(map[string]any)
			require.Equal(t, tc.valid, checkRPCWireSchema(schema, predicate, schemas) == nil)
		})
	}
}

func TestOpenRPCMethodsParametersAndRequirednessMatchHandlers(t *testing.T) {
	document := apidoc.OpenRPCDocument()
	methods := document["methods"].([]apidoc.RPCMethodDescription)
	names := make([]string, 0, len(methods))
	handler, _ := newRPCTestHandler(t)
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
				assert.Equal(t, rpc.MethodParamNames[method.Name], documented)
			} else {
				assert.Contains(t, []string{"deleteSpansByTraceID", "deleteLogsByRefs"}, method.Name)
				assert.Contains(t, method.Description, "Variadic positional")
			}
			if len(required) > 0 {
				_, err := handler.Handle(context.Background(), createRPCRequest(method.Name, required[:len(required)-1]))
				assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams, "missing required positional slot must fail before reading the store")
			}
			assert.NotEmpty(t, method.Description)
			assert.NotEmpty(t, method.Result.Schema)
		})
	}
	assert.ElementsMatch(t, dispatchedRPCMethods(), names)
	assert.Len(t, names, len(mapMethodNames(methods)))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"tags"`)
	assert.Equal(t, 1, strings.Count(string(encoded), "Methods ending in View return UI-shaped data intended for rendering in the viewer."))
}

func mapMethodNames(methods []apidoc.RPCMethodDescription) map[string]bool {
	names := make(map[string]bool, len(methods))
	for _, method := range methods {
		names[method.Name] = true
	}
	return names
}

func TestOpenRPCExamplesExecuteExactly(t *testing.T) {
	handler, _ := newRPCTestHandler(t)
	for _, method := range apidoc.OpenRPCDocument()["methods"].([]apidoc.RPCMethodDescription) {
		for _, example := range method.Examples {
			t.Run(method.Name, func(t *testing.T) {
				named := map[string]any{}
				positional := []any{}
				for _, param := range example["params"].([]map[string]any) {
					name := param["name"].(string)
					assert.Contains(t, rpc.MethodParamNames[method.Name], name)
					named[name] = param["value"]
					positional = append(positional, param["value"])
				}
				want, err := json.Marshal(example["result"].(map[string]any)["value"])
				require.NoError(t, err)
				for _, params := range []any{named, positional} {
					got, err := handler.Handle(context.Background(), createRPCRequest(method.Name, params))
					require.NoError(t, err)
					encoded, err := json.Marshal(got)
					require.NoError(t, err)
					var value any
					require.NoError(t, rpc.DecodeParams(encoded, &value))
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
	handler, _ := newRPCTestHandlerWithData(t)
	handler.SetOTLPHTTPPort(4318)
	result, err := handler.Handle(context.Background(), createRPCRequest("query", []any{`select m.id::varchar, s.id::varchar, l.id::varchar, (select ref from search_services limit 1)
		from metrics m join metric_series s on s.metric_id = m.id cross join logs l limit 1`}))
	require.NoError(t, err)
	var refs struct {
		Rows [][]string `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(result.(json.RawMessage), &refs))
	require.Len(t, refs.Rows, 1)
	require.Len(t, refs.Rows[0], 4)
	metricRef, seriesRef, logRef := refs.Rows[0][0], refs.Rows[0][1], refs.Rows[0][2]
	document := apidoc.OpenRPCDocument()
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	for _, tc := range []struct {
		method string
		params any
	}{
		{"getTraceView", []any{testOpenRPCTraceID}},
		{"getTraceOverview", []any{testOpenRPCTraceID}},
		{"getSpan", []any{"0000000000000001", testOpenRPCTraceID}},
		{"query", []any{"select 9007199254740993::bigint as exact"}},
		{"searchTraceSummaries", []any{nil, nil}},
		{"searchLogSummaries", []any{nil, nil}},
		{"searchLogPage", []any{nil, nil, nil, nil, nil, 1, 0, []any{
			map[string]string{"source": "log", "kind": "field", "key": "timeUnixNano"},
			map[string]string{"source": "log", "kind": "attribute", "key": "missing.attribute"},
		}}},
		{"searchMetricSummaries", []any{nil, nil}},
		{"getTraceLogSummaries", []any{testOpenRPCTraceID}},
		{"getLog", []any{logRef}},
		{"getMetric", []any{metricRef}},
		{"getMetricSeries", []any{metricRef, seriesRef, nil, nil}},
		{"getMetricView", []any{metricRef, nil, nil}},
		{"getMetricAggregateView", []any{metricRef, nil, nil}},
		{"getTraceAttributeDefinitions", []any{}},
		{"getLogAttributeDefinitions", []any{}},
		{"getMetricAttributeDefinitions", []any{}},
		{"getTraceAttributeDefinitionsByTraceID", []any{testOpenRPCTraceID}},
		{"getFieldValueCompletions", []any{"traces", "name", "", 10}},
		{"searchAttributeMatches", []any{"pumpkin", "traces", "resource", []string{"attributes", "service.name"}, "string"}},
		{"getServiceLabels", []any{refs.Rows[0][3]}},
		{"getStats", []any{}},
		{"listDatabases", []any{}},
		{"getTraceSpanCount", []any{testOpenRPCTraceID}},
		{"getImportConfig", []any{}},
		// Mutations run last against this test-owned store only.
		{"deleteMetric", []any{metricRef}},
		{"deleteSpansByTraceID", []any{testOpenRPCTraceID}},
		{"deleteLogsByRefs", []any{logRef}},
		{"clearTraces", []any{}},
		{"clearLogs", []any{}},
		{"clearMetrics", []any{}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRPCRequest(tc.method, tc.params))
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var value any
			require.NoError(t, rpc.DecodeParams(encoded, &value))
			if tc.method == "searchAttributeMatches" {
				require.NotEmpty(t, value)
			}
			if tc.method == "searchLogPage" {
				page := value.(map[string]any)["logs"].([]any)
				require.Len(t, page, 1)
				columns := page[0].(map[string]any)["columnValues"].([]any)
				require.Len(t, columns, 2)
				require.Equal(t, false, columns[1].(map[string]any)["present"])
			}
			for _, method := range document["methods"].([]apidoc.RPCMethodDescription) {
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
	// Confirm serialization does not round the schema's numeric maximum.
	encoded, err := json.Marshal(apidoc.OpenRPCDocument())
	require.NoError(t, err)
	assert.True(t, bytes.Contains(encoded, []byte(`"maximum":18446744073709551615`)))
}
