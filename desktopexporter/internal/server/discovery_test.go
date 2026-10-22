package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

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
			return spans.Ingest(ctx, conn, traces, h.store.FlushedIDs())
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
	require.JSONEq(t, `{"value":{"kind":"int64","value":"9007199254740993"},"foundOn":[{"signal":"traces","ownerType":"span"}],"count":1,"denominator":2,"relativeFrequency":0.5}`, string(values.Rows[0][0]))
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
	} {
		_, err := h.Handle(context.Background(), createRequest("discoverTelemetry", []any{options}))
		require.Error(t, err, "%v", options)
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
