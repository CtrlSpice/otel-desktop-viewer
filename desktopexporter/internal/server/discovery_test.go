package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap/zaptest"
)

func TestDiscoveryQueryResultColumns(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	for _, populated := range []bool{false, true} {
		if populated {
			require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
				return spans.Ingest(ctx, conn, buildTestTraces())
			}))
		}
		for _, kind := range []string{"services", "keys", "values"} {
			options := map[string]any{"kind": kind, "signal": "traces", "ownerType": "span", "key": "http.method"}
			for _, filtered := range []bool{false, true} {
				if filtered {
					options["queries"] = map[string]any{}
				}
				result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
				require.NoError(t, err)
				var page map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
				require.JSONEq(t, `[{"name":"entry","type":"JSON"}]`, string(page["columns"]), "kind=%s filtered=%t populated=%t", kind, filtered, populated)
				require.Contains(t, page, "rows")
				require.Contains(t, page, "truncated")
			}
		}
	}
}

func TestDiscoveryNULStrings(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	data := buildTestTraces()
	resource := data.ResourceSpans().At(0)
	resource.Resource().Attributes().PutStr("service.name", "svc\x00'雪")
	resource.Resource().Attributes().PutStr("service.namespace", "ns\x00")
	span := resource.ScopeSpans().At(0).Spans().At(0)
	span.Attributes().PutStr("a\x00'b", "value\x00'雪")
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))
	for _, kind := range []string{"services", "keys", "values"} {
		options := map[string]any{"kind": kind, "signal": "traces", "ownerType": "span", "key": "a\x00'b", "serviceName": "svc\x00'雪", "namespace": "ns\x00", "term": "\x00"}
		if kind == "values" {
			// Value filtering searches the received JSON representation.
			options["term"] = `\u0000`
		}
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err, kind)
		var page struct {
			Rows [][]json.RawMessage `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
		require.NotEmpty(t, page.Rows, kind)
		if kind == "values" {
			require.Contains(t, string(page.Rows[0][0]), `value\u0000'雪`)
		}
	}
}

func TestServiceIdentityRetainsTypesAcrossDiscoveryAndSignalSearch(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	ctx := context.Background()
	values := []any{"7", int64(7), true, float64(7), math.Copysign(0, -1), float64(0), []any{"7"}, map[string]any{"a": "7"}, nil}
	for index, value := range values {
		attrs := pcommon.NewMap()
		require.NoError(t, attrs.PutEmpty("service.name").FromRaw(value))
		require.NoError(t, attrs.PutEmpty("service.namespace").FromRaw(value))
		data := buildTestTraces()
		r := data.ResourceSpans().At(0)
		attrs.CopyTo(r.Resource().Attributes())
		s := r.ScopeSpans().At(0).Spans().At(0)
		s.SetTraceID([16]byte{byte(index + 1)})
		s.SetStartTimestamp(100)
		s.Attributes().PutInt("identity-index", int64(index))
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
		ld := buildTestLogs()
		attrs.CopyTo(ld.ResourceLogs().At(0).Resource().Attributes())
		ld.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).SetTimestamp(100)
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, ld) }))
		md := buildTestMetrics()
		attrs.CopyTo(md.ResourceMetrics().At(0).Resource().Attributes())
		md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).SetTimestamp(100)
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, md, h.store.FlushedIDs()) }))
	}
	result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "services"}}))
	require.NoError(t, err)
	var page struct {
		Rows [][]struct {
			Identity    string `json:"serviceIdentity"`
			SpanCount   int    `json:"spanCount"`
			LogCount    int    `json:"logCount"`
			MetricCount int    `json:"metricCount"`
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
	require.Len(t, page.Rows, len(values))
	for _, row := range page.Rows {
		service := row[0]
		require.Equal(t, 1, service.SpanCount)
		require.Equal(t, 1, service.LogCount)
		require.Equal(t, 1, service.MetricCount)
		query := map[string]any{"type": "condition", "query": map[string]any{"field": map[string]any{"name": "serviceIdentity", "searchScope": "field", "type": "string"}, "fieldOperator": "=", "value": service.Identity}}
		for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
			result, err := h.Handle(ctx, createRequest(method, []any{nil, nil, query}))
			require.NoError(t, err, method)
			var rows []json.RawMessage
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &rows))
			require.Len(t, rows, 1, method)
		}
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "values", "signal": "traces", "ownerType": "span", "key": "identity-index", "serviceIdentity": service.Identity}}))
		require.NoError(t, err)
		var valuesPage struct {
			Rows [][]json.RawMessage `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &valuesPage))
		require.Len(t, valuesPage.Rows, 1)
		require.Contains(t, string(valuesPage.Rows[0][0]), `"denominator":1`)
	}
}

func TestHomeSearchCountsBeforePaginationAndFiltersAttributes(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	ctx := context.Background()
	for index, name := range []string{"api", "worker", "unmatched"} {
		data := buildTestTraces()
		r := data.ResourceSpans().At(0)
		r.Resource().Attributes().PutStr("service.name", name)
		s := r.ScopeSpans().At(0).Spans().At(0)
		s.SetSpanID([8]byte{byte(index + 1)})
		s.SetStartTimestamp(100)
		s.SetName("needle")
		s.Attributes().PutStr("matched", name)
		if index == 2 {
			s.SetName("other")
			s.Attributes().PutStr("excluded", "yes")
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
	}
	ld := buildTestLogs()
	l := ld.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	l.SetTimestamp(0)
	l.SetObservedTimestamp(100)
	l.Body().SetStr("needle")
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, ld) }))
	md := buildTestMetrics()
	m := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
	m.SetName("needle")
	m.Gauge().DataPoints().At(0).SetTimestamp(100)
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, md, h.store.FlushedIDs()) }))
	predicate := map[string]any{"type": "condition", "query": map[string]any{"field": map[string]any{"searchScope": "global"}, "fieldOperator": "CONTAINS", "value": "needle"}}
	queries := map[string]any{"traces": predicate, "logs": predicate, "metrics": predicate}
	options := map[string]any{"kind": "services", "queries": queries, "startTime": "100", "endTime": "100", "limit": 1}
	read := func() map[string]json.RawMessage {
		t.Helper()
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var page map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
		return page
	}
	page := read()
	require.JSONEq(t, `{"home":4,"traces":1,"logs":1,"metrics":1}`, string(page["totals"]))
	require.Equal(t, "true", string(page["truncated"]))
	require.Contains(t, string(page["rows"]), `"serviceName":"api"`)
	options["offset"] = 3
	page = read()
	require.JSONEq(t, `{"home":4,"traces":1,"logs":1,"metrics":1}`, string(page["totals"]))
	require.Equal(t, "false", string(page["truncated"]))
	options["offset"] = 20
	page = read()
	require.JSONEq(t, `[]`, string(page["rows"]))
	require.Contains(t, string(page["totals"]), `"home":4`)
	options["offset"] = 0
	options["limit"] = 100
	options["kind"] = "keys"
	options["signal"] = "traces"
	options["ownerType"] = "span"
	page = read()
	require.Contains(t, string(page["rows"]), `"matched"`)
	require.NotContains(t, string(page["rows"]), `"excluded"`)
	options["kind"] = "values"
	options["key"] = "matched"
	page = read()
	require.Contains(t, string(page["rows"]), `"denominator":2`)
	queries["traces"] = false
	require.JSONEq(t, `[]`, string(read()["rows"]))
}

func TestDiscoveryNamespaceValuesAndPagination(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	for index, namespace := range []string{"storefront", "internal", ""} {
		traces := buildTestTraces()
		resource := traces.ResourceSpans().At(0)
		if namespace != "" {
			resource.Resource().Attributes().PutStr("service.namespace", namespace)
		}
		span := resource.ScopeSpans().At(0).Spans().At(0)
		span.SetSpanID([8]byte{byte(index + 1)})
		span.SetStartTimestamp(pcommon.Timestamp(100))
		span.Attributes().PutInt("exact'key", 9007199254740993)
		if index == 0 {
			second := resource.ScopeSpans().At(0).Spans().AppendEmpty()
			span.CopyTo(second)
			second.SetSpanID([8]byte{42})
			second.Attributes().PutStr("exact'key", "9007199254740993")
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return spans.Ingest(ctx, conn, traces)
		}))
	}
	type page struct {
		Rows      [][]json.RawMessage `json:"rows"`
		Truncated bool                `json:"truncated"`
	}
	discover := func(options map[string]any) page {
		t.Helper()
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var got page
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &got))
		return got
	}
	services := discover(map[string]any{"kind": "services", "limit": 1})
	require.True(t, services.Truncated)
	require.Len(t, services.Rows, 1)
	require.Contains(t, string(services.Rows[0][0]), `"serviceNamespace":""`)
	services = discover(map[string]any{"kind": "services", "limit": 1, "offset": 2})
	require.False(t, services.Truncated)
	require.Contains(t, string(services.Rows[0][0]), `"serviceNamespace":"storefront"`)
	require.Contains(t, string(services.Rows[0][0]), `"spanCount":2`)
	options := map[string]any{"kind": "values", "serviceName": "pumpkin.pie", "namespace": "storefront", "signal": "traces", "ownerType": "span", "key": "exact'key", "limit": 1}
	values := discover(options)
	require.True(t, values.Truncated)
	require.JSONEq(t, `{"value":{"kind":"int64","value":"9007199254740993"},"foundOn":[{"signal":"traces","ownerType":"span"}],"count":1,"kindCount":1,"denominator":2,"relativeFrequency":0.5}`, string(values.Rows[0][0]))
	options["namespace"] = ""
	values = discover(options)
	require.False(t, values.Truncated)
	require.Contains(t, string(values.Rows[0][0]), `"denominator":1`)
	options["startTime"] = "101"
	require.Empty(t, discover(options).Rows)
	options["startTime"] = "100"
	options["endTime"] = "100"
	require.Len(t, discover(options).Rows, 1)
	options["namespace"] = "' OR true --"
	require.Empty(t, discover(options).Rows)
}

func TestDiscoveryRejectsInvalidOptions(t *testing.T) {
	h := setupHandler(t)
	for _, options := range []any{nil, true, map[string]any{},
		map[string]any{"kind": "services", "limit": 0},
		map[string]any{"kind": "services", "startTime": "-1"},
		map[string]any{"kind": "keys", "signal": "logs", "ownerType": "span"},
		map[string]any{"kind": "values", "signal": "traces", "ownerType": "resource"},
		map[string]any{"kind": "services", "unexpected": true},
		map[string]any{"kind": "services", "sortBy": "name"},
		map[string]any{"kind": "values", "signal": "traces", "ownerType": "span", "key": "name", "sortBy": "count"},
		map[string]any{"kind": "values", "signal": "traces", "ownerType": "span", "key": "name", "sortDirection": "asc; SELECT 1"},
	} {
		_, err := h.Handle(context.Background(), createRequest("discoverTelemetry", []any{options}))
		require.Error(t, err, "%v", options)
	}
}

func TestDiscoveryValueSortAcrossPages(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	for index, name := range []string{"zebra", "zebra", "zebra", "apple", "apple", "mango", "mango"} {
		traces := buildTestTraces()
		span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		span.SetSpanID([8]byte{byte(index + 1)})
		span.Attributes().PutStr("sortable", name)
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return spans.Ingest(ctx, conn, traces)
		}))
	}
	for _, tc := range []struct {
		by, direction string
		want          []string
	}{
		{"name", "asc", []string{"apple", "mango", "zebra"}},
		{"name", "desc", []string{"zebra", "mango", "apple"}},
		{"frequency", "asc", []string{"apple", "mango", "zebra"}},
		{"frequency", "desc", []string{"zebra", "apple", "mango"}},
	} {
		t.Run(tc.by+"-"+tc.direction, func(t *testing.T) {
			for offset, want := range tc.want {
				result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{
					"kind": "values", "signal": "traces", "ownerType": "span", "key": "sortable",
					"sortBy": tc.by, "sortDirection": tc.direction, "limit": 1, "offset": offset,
				}}))
				require.NoError(t, err)
				var page struct {
					Rows [][]struct {
						Value struct {
							Value string `json:"value"`
						} `json:"value"`
						Denominator int `json:"denominator"`
					} `json:"rows"`
					Truncated bool `json:"truncated"`
				}
				require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
				require.Len(t, page.Rows, 1)
				require.Equal(t, want, page.Rows[0][0].Value.Value)
				require.Equal(t, 7, page.Rows[0][0].Denominator)
				require.Equal(t, offset < 2, page.Truncated)
			}
		})
	}
}

func TestHomeLinksMatchMissingNamespaceAcrossSignals(t *testing.T) {
	h := setupHandlerWithData(t)
	ctx := context.Background()
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTestMetrics(), h.store.FlushedIDs())
	}))
	for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
		for _, key := range []string{"service.namespace", "service.name"} {
			query := map[string]any{"type": "condition", "query": map[string]any{
				"field":         map[string]any{"name": key, "type": "string", "searchScope": "attribute", "attributeScope": "resource"},
				"fieldOperator": "IS NULL", "value": "",
			}}
			result, err := h.Handle(ctx, createRequest(method, map[string]any{"startTime": nil, "endTime": nil, "query": query}))
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var rows []json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &rows))
			if key == "service.namespace" {
				require.NotEmpty(t, rows, method)
			} else {
				require.Empty(t, rows, method)
			}
		}
	}
}
