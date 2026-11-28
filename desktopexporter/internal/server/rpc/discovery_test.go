package rpc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
		wire, err := json.Marshal(map[string]any{"options": options})
		require.NoError(t, err)
		native := readArrowDataset(t, h, "discoverTelemetry", string(wire))
		require.Equal(t, len(page.Rows), native.Column(0).(*array.List).ListValues().Len())
	}
}

func TestArrowDiscoveryReceivedValues(t *testing.T) {
	h := setupHandler(t)
	values := []any{int64(9007199254740993), float64(0), math.Copysign(0, -1), math.Float64frombits(0x7ff8000000000001), math.Float64frombits(0x7ff8000000000002), []any{int64(9007199254740993)}, nil}
	for index, value := range values {
		data := buildTestTraces()
		span := data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		span.SetTraceID(pcommon.TraceID{15: byte(index + 1)})
		require.NoError(t, span.Attributes().PutEmpty("exact").FromRaw(value))
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	}
	params := map[string]any{"options": map[string]any{"kind": "values", "signal": "traces", "ownerType": "span", "key": "exact", "sortBy": "name", "sortDirection": "desc"}}
	legacy, err := h.Handle(t.Context(), createRequest("discoverTelemetry", params))
	require.NoError(t, err)
	var page struct {
		Rows [][]struct {
			Value json.RawMessage `json:"value"`
		}
	}
	require.NoError(t, json.Unmarshal(legacy.(json.RawMessage), &page))
	wire, err := json.Marshal(params)
	require.NoError(t, err)
	native := readArrowDataset(t, h, "discoverTelemetry", string(wire))
	items := native.Column(0).(*array.List).ListValues().(*array.Struct)
	// Discovery groups NaNs by canonical content identity, as the JSON endpoint
	// does. The representative native value must still retain received bits.
	require.Equal(t, len(values)-1, items.Len())
	require.Len(t, page.Rows, items.Len())
	index, ok := items.DataType().(*arrow.StructType).FieldIdx("identity")
	require.True(t, ok)
	identities := items.Field(index).(*array.String)
	index, ok = items.DataType().(*arrow.StructType).FieldIdx("value")
	require.True(t, ok)
	variants := items.Field(index)
	for row := range items.Len() {
		require.JSONEq(t, string(page.Rows[row][0].Value), identities.Value(row))
		var identity struct {
			Kind  string          `json:"kind"`
			Value json.RawMessage `json:"value"`
		}
		require.NoError(t, json.Unmarshal([]byte(identities.Value(row)), &identity))
		value := readArrowVariant(t, variants, row)
		switch identity.Kind {
		case "int64":
			require.Equal(t, int64(9007199254740993), value.Value())
		case "double":
			var expected float64
			if string(identity.Value) == "0" {
				expected = 0
			} else {
				var text string
				require.NoError(t, json.Unmarshal(identity.Value, &text))
				bits, err := strconv.ParseUint(strings.TrimPrefix(text, "0x"), 16, 64)
				require.NoError(t, err)
				expected = math.Float64frombits(bits)
			}
			if math.IsNaN(expected) {
				expected = math.Float64frombits(0x7ff8000000000001)
			}
			require.Equal(t, math.Float64bits(expected), math.Float64bits(value.Value().(float64)))
		case "array":
			child, err := value.Value().(pv.ArrayValue).Value(0)
			require.NoError(t, err)
			require.Equal(t, int64(9007199254740993), child.Value())
		case "empty":
			require.Equal(t, pv.Null, value.Type())
		default:
			t.Fatalf("unexpected received kind %s", identity.Kind)
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
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, md) }))
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
	count, err := h.Handle(ctx, createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, len(values), count)
	for _, row := range page.Rows {
		service := row[0]
		filtered, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "services", "serviceIdentity": service.Identity}}))
		require.NoError(t, err)
		var filteredPage struct {
			Rows [][]json.RawMessage `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(filtered.(json.RawMessage), &filteredPage))
		require.Len(t, filteredPage.Rows, 1)
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
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, md) }))
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
		return metrics.Ingest(ctx, conn, buildTestMetrics())
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

func TestDiscoveryKindCountsDistinctRecordsWithinOwnerAndSignal(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	data := ptrace.NewTraces()
	for index := range 103 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", "checkout")
		scope := r.ScopeSpans().AppendEmpty()
		s := scope.Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{15: 1})
		s.SetSpanID(pcommon.SpanID{7: byte(index + 1)})
		s.SetStartTimestamp(100)
		s.SetEndTimestamp(101)
		s.SetName("needle")
		s.Attributes().PutStr("attempt", "1")
		if index == 99 {
			s.Attributes().PutInt("attempt", 1)
		}
		if index == 100 {
			r.Resource().Attributes().PutStr("service.name", "other")
		}
		if index == 101 {
			s.SetStartTimestamp(101)
		}
		if index == 102 {
			s.SetName("other")
		}
		if index < 3 {
			scope.Scope().Attributes().PutStr("attempt", "1")
		} else if index == 3 {
			scope.Scope().Attributes().PutInt("attempt", 1)
		}
		if index < 3 {
			values := [][]string{{"1", "2", "1"}, {"1"}, {"2"}}
			for _, value := range values[index] {
				event := s.Events().AppendEmpty()
				event.SetTimestamp(100)
				event.Attributes().PutStr("attempt", value)
			}
		}
		// One span carries both kinds; neither kind counts nor value counts
		// can be summed to recover the all-kind distinct-record denominator.
		if index == 0 || index == 3 {
			event := s.Events().AppendEmpty()
			event.SetTimestamp(100)
			event.Attributes().PutInt("attempt", 1)
		}
		if index == 0 {
			for _, value := range []string{"1", "2", "1"} {
				s.Links().AppendEmpty().Attributes().PutStr("attempt", value)
			}
		} else if index < 3 {
			s.Links().AppendEmpty().Attributes().PutInt("attempt", 1)
		}
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))
	for index := range 3 {
		ld := buildTestLogs()
		r := ld.ResourceLogs().At(0)
		r.Resource().Attributes().PutStr("service.name", "checkout")
		scope := r.ScopeLogs().At(0)
		scope.LogRecords().At(0).SetTimestamp(100)
		if index == 0 {
			scope.Scope().Attributes().PutStr("attempt", "1")
		} else {
			scope.Scope().Attributes().PutInt("attempt", 1)
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return logs.Ingest(ctx, conn, ld)
		}))
		md := buildTestMetrics()
		rm := md.ResourceMetrics().At(0)
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		sm := rm.ScopeMetrics().At(0)
		sm.Metrics().At(0).Gauge().DataPoints().At(0).SetTimestamp(100)
		if index < 2 {
			sm.Scope().Attributes().PutStr("attempt", "1")
		} else {
			sm.Scope().Attributes().PutInt("attempt", 1)
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
	}
	for _, tc := range []struct {
		signal, ownerType      string
		strings, ints, records int
		values                 int
	}{
		{"traces", "span", 99, 1, 100, 2},
		{"traces", "event", 3, 2, 4, 3},
		{"traces", "link", 1, 2, 3, 3},
		{"traces", "scope", 3, 1, 4, 2},
		{"logs", "scope", 1, 2, 3, 2},
		{"metrics", "scope", 2, 1, 3, 2},
	} {
		t.Run(tc.signal+"/"+tc.ownerType, func(t *testing.T) {
			for offset := range tc.values {
				params := map[string]any{"options": map[string]any{
					"kind": "values", "signal": tc.signal, "ownerType": tc.ownerType, "key": "attempt",
					"serviceName": "checkout", "startTime": "100", "endTime": "100", "limit": 1, "offset": offset,
					"queries": map[string]any{"traces": map[string]any{"type": "condition", "query": map[string]any{
						"field": map[string]any{"name": "name", "searchScope": "field", "type": "string"}, "fieldOperator": "=", "value": "needle",
					}}},
				}}
				result, err := h.Handle(ctx, createRequest("discoverTelemetry", params))
				require.NoError(t, err)
				var page struct {
					Rows [][]struct {
						Value struct {
							Kind string `json:"kind"`
						} `json:"value"`
						Count             int                  `json:"count"`
						KindCount         int                  `json:"kindCount"`
						Denominator       int                  `json:"denominator"`
						RelativeFrequency float64              `json:"relativeFrequency"`
						FoundOn           []discovery.Location `json:"foundOn"`
					} `json:"rows"`
					Truncated bool `json:"truncated"`
				}
				require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
				require.Len(t, page.Rows, 1)
				value := page.Rows[0][0]
				want := tc.strings
				if value.Value.Kind == "int64" {
					want = tc.ints
				} else {
					require.Equal(t, "string", value.Value.Kind)
				}
				require.Equal(t, want, value.KindCount)
				valueCount := want
				if tc.ownerType == "event" && value.Value.Kind == "string" {
					valueCount = 2 // Two values overlap on one of three spans.
				}
				require.Equal(t, valueCount, value.Count)
				require.Equal(t, tc.records, value.Denominator)
				require.Equal(t, float64(valueCount)/float64(tc.records), value.RelativeFrequency)
				require.Equal(t, []discovery.Location{{Signal: tc.signal, OwnerType: tc.ownerType}}, value.FoundOn)
				require.Equal(t, offset < tc.values-1, page.Truncated)
				wire, err := json.Marshal(params)
				require.NoError(t, err)
				native := readArrowDataset(t, h, "discoverTelemetry", string(wire))
				items := native.Column(0).(*array.List).ListValues().(*array.Struct)
				require.Equal(t, 1, items.Len())
				field := func(name string) arrow.Array {
					index, ok := items.DataType().(*arrow.StructType).FieldIdx(name)
					require.True(t, ok)
					return items.Field(index)
				}
				require.Equal(t, int64(value.Count), field("count").(*array.Int64).Value(0))
				require.Equal(t, int64(value.KindCount), field("kindCount").(*array.Int64).Value(0))
				require.Equal(t, int64(value.Denominator), field("denominator").(*array.Int64).Value(0))
				require.Equal(t, value.RelativeFrequency, field("relativeFrequency").(*array.Float64).Value(0))
				var identity struct {
					Kind string `json:"kind"`
				}
				require.NoError(t, json.Unmarshal([]byte(field("identity").(*array.String).Value(0)), &identity))
				require.Equal(t, value.Value.Kind, identity.Kind)
				require.Equal(t, page.Truncated, native.Column(1).(*array.Boolean).Value(0))
				// Home uses one cross-signal page. Restrict this case to the same
				// selected signal and pin the existing parent/kind denominators.
				homeOptions := map[string]any{}
				for key, value := range params["options"].(map[string]any) {
					homeOptions[key] = value
				}
				queries := map[string]any{"traces": false, "metrics": false, "logs": false}
				queries[tc.signal] = nil
				if tc.signal == "traces" {
					queries[tc.signal] = params["options"].(map[string]any)["queries"].(map[string]any)["traces"]
				}
				homeOptions["queries"] = queries
				home := homePage(t, h, homeOptions)
				require.Len(t, home.Values, 1)
				homeValue := home.Values[0]
				homeKindCount := tc.strings
				if homeValue.Kind == "int64" {
					homeKindCount = tc.ints
				}
				homeCount := homeKindCount
				if tc.ownerType == "event" && homeValue.Kind == "string" {
					homeCount = 2
				}
				require.EqualValues(t, homeKindCount, homeValue.KindCount)
				require.EqualValues(t, homeCount, homeValue.Count)
				require.EqualValues(t, tc.records, home.Denominator)
				require.EqualValues(t, tc.values, home.Total)
				require.Equal(t, offset < tc.values-1, home.Truncated)
			}
		})
	}
}

func TestServicesGroupResourcesWithoutSplittingScopesOrDoubleCountingTraces(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for resourceIndex := range 2 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", "shared")
		r.Resource().Attributes().PutInt("unrelated", int64(resourceIndex))
		if resourceIndex == 1 {
			r.Resource().Attributes().PutStr("service.namespace", "")
		}
		for scopeIndex := range 2 {
			scope := r.ScopeSpans().AppendEmpty()
			scope.Scope().SetName(fmt.Sprint(scopeIndex))
			// Scope-owned service-looking keys must not become Resource identity.
			scope.Scope().Attributes().PutStr("service.name", fmt.Sprint(scopeIndex))
			for i := range 100 {
				span := scope.Spans().AppendEmpty()
				span.SetTraceID(pcommon.TraceID{15: 1})
				id := resourceIndex*200 + scopeIndex*100 + i + 1
				span.SetSpanID(pcommon.SpanID{6: byte(id >> 8), 7: byte(id)})
				span.SetStartTimestamp(pcommon.Timestamp(i))
				span.SetEndTimestamp(pcommon.Timestamp(i + 1))
			}
		}
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	for _, window := range []bool{false, true} {
		options := map[string]any{"kind": "services", "queries": map[string]any{}}
		want := 400
		if window {
			options["startTime"], options["endTime"] = "10", "19"
			want = 40
		}
		raw, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var page struct {
			Rows [][]struct {
				Spans  int `json:"spanCount"`
				Traces int `json:"traceCount"`
			} `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(raw.(json.RawMessage), &page))
		require.Len(t, page.Rows, 1)
		require.Equal(t, want, page.Rows[0][0].Spans)
		require.Equal(t, 1, page.Rows[0][0].Traces)
	}
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	// Reads aggregate compact resource IDs. The catalogue holds the typed pair
	// once despite unrelated attributes, scopes and hundreds of owning records.
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var key, raw string
		var builder discovery.Builder
		statement := builder.Services(discovery.Query{})
		err := db.QueryRowContext(t.Context(), "explain (analyze, format json) "+statement, builder.Args()...).Scan(&key, &raw)
		if err != nil {
			return err
		}
		require.Contains(t, raw, "search_service_resources")
		var services, resources int
		require.NoError(t, db.QueryRow(`select (select count(*) from search_services),(select count(*) from search_service_resources)`).Scan(&services, &resources))
		require.Equal(t, 1, services)
		require.Equal(t, 2, resources)
		require.NotContains(t, raw, "otel_native_text")
		return nil
	}))
}

func TestServiceCountIsScalarBeyondOneSummaryPage(t *testing.T) {
	h := setupHandler(t)
	empty, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 0, empty)
	data := ptrace.NewTraces()
	for i := range 105 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", fmt.Sprint(i))
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 105, count)
	for _, params := range []any{[]any{1}, map[string]any{"limit": 1}} {
		_, err := h.Handle(t.Context(), createRequest("getServiceCount", params))
		require.Error(t, err)
	}
}

func TestServiceCatalogueBoundsLabelsButKeepsExactSelectionAndCopy(t *testing.T) {
	h := setupHandler(t)
	prefix := strings.Repeat("雪'quoted\"\\", 2000)
	data := ptrace.NewTraces()
	for i, suffix := range []string{"z-tail", "a-tail"} {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", prefix+suffix)
		r.Resource().Attributes().PutStr("service.namespace", prefix)
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{7: 1})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	read := func(options string) *array.Struct {
		record := readArrowDataset(t, h, "discoverTelemetry", `{"options":`+options+`}`)
		return record.Column(0).(*array.List).ListValues().(*array.Struct)
	}
	field := func(items *array.Struct, name string) *array.String {
		index, ok := items.DataType().(*arrow.StructType).FieldIdx(name)
		require.True(t, ok)
		return items.Field(index).(*array.String)
	}
	items := read(`{"kind":"services","limit":1}`)
	require.Equal(t, 1, items.Len())
	ref := field(items, "serviceIdentity").Value(0)
	require.Len(t, ref, 64)
	require.LessOrEqual(t, len([]rune(field(items, "serviceName").Value(0))), 300)
	require.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(prefix+"a-tail"))), field(items, "serviceColorKey").Value(0))
	labels, err := h.Handle(t.Context(), createRequest("getServiceLabels", map[string]any{"serviceRef": ref}))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"serviceName": prefix + "a-tail", "serviceNamespace": prefix}, labels)
	second := read(`{"kind":"services","offset":1,"limit":1}`)
	require.NotEqual(t, ref, field(second, "serviceIdentity").Value(0))
	require.Equal(t, field(items, "serviceName").Value(0), field(second, "serviceName").Value(0))
	filtered := read(`{"kind":"services","term":"a-tail"}`)
	require.Equal(t, 1, filtered.Len())
	require.Equal(t, ref, field(filtered, "serviceIdentity").Value(0))
	selected := read(`{"kind":"services","serviceIdentity":"` + ref + `"}`)
	require.Equal(t, 1, selected.Len())
	_, err = h.Handle(t.Context(), createRequest("deleteSpansByTraceID", []any{"00000000000000000000000000000002"}))
	require.NoError(t, err)
	_, err = h.Handle(t.Context(), createRequest("getServiceLabels", map[string]any{"serviceRef": ref}))
	require.ErrorIs(t, err, ErrHomeValueUnavailable)
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestServiceCatalogueCanonicalNaNsAcrossDistinctResources(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for i, bits := range []uint64{0x7ff8000000000001, 0x7ff8000000000002, 0xfff8000000000001} {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutDouble("service.name", math.Float64frombits(bits))
		r.Resource().Attributes().PutEmptyMap("service.namespace").PutDouble("nested", math.Float64frombits(bits))
		r.Resource().Attributes().PutInt("unrelated", int64(i))
		span := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	count, err := h.Handle(t.Context(), createRequest("getServiceCount", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var services, resources int
		require.NoError(t, db.QueryRow(`select (select count(*) from search_services),(select count(*) from search_service_resources)`).Scan(&services, &resources))
		require.Equal(t, 1, services)
		require.Equal(t, 3, resources)
		return nil
	}))
}

func TestServiceMapConnections(t *testing.T) {
	h := setupHandler(t)
	h.logger = zaptest.NewLogger(t)
	ctx := context.Background()
	data := ptrace.NewTraces()
	add := func(service, namespace string, trace, id, parent byte, start uint64) ptrace.Span {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", service)
		r.Resource().Attributes().PutStr("service.namespace", namespace)
		s := r.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{trace})
		s.SetSpanID(pcommon.SpanID{id})
		if parent != 0 {
			s.SetParentSpanID(pcommon.SpanID{parent})
		}
		s.SetName(service)
		s.SetStartTimestamp(pcommon.Timestamp(start))
		s.SetEndTimestamp(pcommon.Timestamp(start + 10))
		return s
	}
	add("api", "one", 1, 1, 0, 100)
	add("worker", "", 1, 2, 1, 110)
	add("worker", "", 1, 3, 1, 120).Status().SetCode(ptrace.StatusCodeError)
	add("api", "one", 1, 4, 2, 130) // reverse edge, not a duplicate node
	add("api", "two", 1, 5, 1, 140) // same name, distinct namespace
	add("orphan", "", 2, 6, 1, 150) // parent ID in another trace is not a connection
	add("api", "one", 1, 7, 1, 160) // same-service span is not an edge
	add("api", "one", 3, 1, 0, 90)
	add("worker", "", 3, 2, 1, 95) // same edge in a second trace
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
	read := func(options map[string]any) struct {
		Rows      [][]json.RawMessage `json:"rows"`
		Truncated bool                `json:"truncated"`
	} {
		t.Helper()
		result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": options}))
		require.NoError(t, err)
		var page struct {
			Rows      [][]json.RawMessage `json:"rows"`
			Truncated bool                `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
		wire, err := json.Marshal(map[string]any{"options": options})
		require.NoError(t, err)
		native := readArrowDataset(t, h, "discoverTelemetry", string(wire))
		require.Equal(t, len(page.Rows), native.Column(0).(*array.List).ListValues().Len())
		require.Equal(t, page.Truncated, native.Column(1).(*array.Boolean).Value(0))
		return page
	}
	// Follow the documented lookup: the service row supplies the identity
	// for connections, even when another namespace has the same service name.
	services := read(map[string]any{"kind": "services", "serviceName": "api", "namespace": "one"})
	require.Len(t, services.Rows, 1)
	var service struct{ ServiceIdentity string }
	require.NoError(t, json.Unmarshal(services.Rows[0][0], &service))
	identity := service.ServiceIdentity
	require.NotEmpty(t, identity)
	options := map[string]any{"kind": "connections", "serviceIdentity": identity, "limit": 1}
	page := read(options)
	require.Len(t, page.Rows, 1)
	require.True(t, page.Truncated)
	options["limit"] = 100
	page = read(options)
	require.Len(t, page.Rows, 3)
	options["offset"] = 1
	require.Equal(t, page.Rows[1:], read(options).Rows)
	delete(options, "offset")
	var workerIdentity string
	for _, row := range page.Rows {
		var edge struct {
			Source struct {
				Identity string `json:"serviceIdentity"`
			} `json:"source"`
			Target struct {
				Name     string `json:"serviceName"`
				Identity string `json:"serviceIdentity"`
			} `json:"target"`
			SpanCount      int `json:"spanCount"`
			TraceCount     int `json:"traceCount"`
			ErrorSpanCount int `json:"errorSpanCount"`
		}
		require.NoError(t, json.Unmarshal(row[0], &edge))
		if edge.Source.Identity == identity && edge.Target.Name == "worker" {
			workerIdentity = edge.Target.Identity
			require.Equal(t, 3, edge.SpanCount)
			require.Equal(t, 2, edge.TraceCount)
			require.Equal(t, 1, edge.ErrorSpanCount)
		}
	}
	require.NotEmpty(t, workerIdentity)
	options["term"] = `"lastSeen":"160"`
	require.NotEmpty(t, read(options).Rows)
	delete(options, "term")
	children := read(map[string]any{"kind": "connection-spans", "serviceIdentity": identity, "targetIdentity": workerIdentity})
	require.Len(t, children.Rows, 3)
	require.Contains(t, string(children.Rows[0][0]), `"spanID":"0200000000000000"`)
	evidence := map[string]any{"kind": "connection-spans", "serviceIdentity": identity, "targetIdentity": workerIdentity, "limit": 1, "offset": 1}
	require.Equal(t, children.Rows[1:2], read(evidence).Rows)
	require.True(t, read(evidence).Truncated)
	evidence["offset"] = 2
	require.False(t, read(evidence).Truncated)
	options["queries"] = map[string]any{"traces": map[string]any{"type": "condition", "query": map[string]any{"field": map[string]any{"searchScope": "global"}, "fieldOperator": "CONTAINS", "value": "worker"}}}
	require.Empty(t, read(options).Rows) // child matches, but its parent does not
	delete(options, "queries")
	options["startTime"] = "110" // excludes the parent: no inferred cross-window edge
	page = read(options)
	require.Len(t, page.Rows, 1)
	options["queries"] = map[string]any{"traces": false}
	require.Empty(t, read(options).Rows)
	_, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connections"}}))
	require.Error(t, err)
	_, err = h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{"kind": "connection-spans", "serviceIdentity": identity}}))
	require.Error(t, err)
}

// Exercise the HTTP-to-generated-SQL paths reported by go/sql-injection.
// Hostile-looking received strings must remain searchable data; identifiers,
// directions and timestamp bounds must never become arbitrary SQL fragments.
func TestDiscoverySQLInputBoundaries(t *testing.T) {
	h := setupHandler(t)
	mux := serveArrowMux(h)
	data := buildTestTraces()
	resource := data.ResourceSpans().At(0)
	span := resource.ScopeSpans().At(0).Spans().At(0)
	const hostile = "' OR true; DROP TABLE spans; --\\雪"
	resource.Resource().Attributes().PutStr("service.name", hostile)
	resource.Resource().Attributes().PutStr("service.namespace", hostile)
	span.Attributes().PutStr(hostile, hostile)
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	for _, kind := range []string{"services", "keys", "values", "home-values"} {
		options := map[string]any{"kind": kind, "signal": "traces", "ownerType": "span", "key": hostile, "serviceName": hostile, "namespace": hostile, "queries": map[string]any{}}
		// JSON value search sees escaped backslashes; this prefix is common to
		// both the native string and its exact JSON identity.
		options["term"] = "' OR true; DROP TABLE spans; --"
		if kind == "home-values" {
			page := homePage(t, h, options)
			require.Len(t, page.Values, 1)
			require.Equal(t, hostile, page.Values[0].Preview)
		} else {
			wire, err := json.Marshal(map[string]any{"options": options})
			require.NoError(t, err)
			native := readArrowDataset(t, h, "discoverTelemetry", string(wire))
			require.Equal(t, 1, native.Column(0).(*array.List).ListValues().Len(), kind)
			result, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": options}))
			require.NoError(t, err)
			var page struct {
				Rows [][]json.RawMessage `json:"rows"`
			}
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
			require.Len(t, page.Rows, 1, kind)
		}
	}
	traceID := span.TraceID().String()
	_, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": map[string]any{
		"kind": "home-values", "ownerType": "span", "key": hostile, "endTime": "0; DROP TABLE spans",
	}}))
	require.Error(t, err)
	for _, method := range []string{"getTraceView", "getTraceColumnPreviews"} {
		wire, err := json.Marshal(map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": "span", "key": hostile}}})
		require.NoError(t, err)
		readArrowDataset(t, h, method, string(wire))
	}
	for _, source := range []string{"log", "resource", "scope"} {
		wire, err := json.Marshal(map[string]any{"startTime": nil, "endTime": nil, "columns": []map[string]string{{"source": source, "kind": "attribute", "key": hostile}}})
		require.NoError(t, err)
		readArrowDataset(t, h, "searchLogPage", string(wire))
	}
	for _, request := range []struct {
		method string
		params map[string]any
	}{
		{"discoverTelemetry", map[string]any{"options": map[string]any{"kind": "keys", "signal": "traces", "ownerType": "event", "startTime": "0' OR true --"}}},
		{"getTraceView", map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": hostile, "key": hostile}}}},
		{"getTraceColumnPreviews", map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": hostile, "key": hostile}}}},
		{"searchTraceSummaries", map[string]any{"startTime": nil, "endTime": nil, "sort": map[string]string{"field": "duration", "direction": "desc; DROP TABLE spans"}}},
		{"searchLogPage", map[string]any{"startTime": nil, "endTime": nil, "columns": []map[string]string{{"source": hostile, "kind": "attribute", "key": hostile}}}},
	} {
		wire, err := json.Marshal(request.params)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/data/"+request.method, strings.NewReader(string(wire))))
		require.Equal(t, http.StatusBadRequest, response.Code, "%s: %s", request.method, response.Body.String())
	}
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var count int
		err := db.QueryRowContext(t.Context(), "select count(*) from spans").Scan(&count)
		require.Positive(t, count)
		return err
	}))
}

func TestDiscoveryPlansSeparateSQLFromValues(t *testing.T) {
	// Changing values of the same query shape must change only its parameters.
	// Include exact uint64 bounds and strings that resemble SQL and placeholders.
	for _, kind := range []string{"services", "keys", "values", "connections", "connection-spans", "home-values"} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%t", kind, native), func(t *testing.T) {
				var previous *discoveryPlan
				for _, value := range []string{"ordinary", "' OR true; -- $1::VARCHAR \\雪\x00"} {
					start, end := "18446744073709551614", "18446744073709551615"
					options := discoveryOptions{
						Kind: kind, Signal: "traces", OwnerType: "event", Key: &value,
						ServiceName: &value, Namespace: &value, ServiceIdentity: &value, TargetIdentity: &value,
						StartTime: &start, EndTime: &end, Term: value,
						Queries: &discoveryQueries{Traces: json.RawMessage(`{"type":"condition","query":{"field":{"searchScope":"global"},"fieldOperator":"CONTAINS","value":"prefix argument"}}`)},
					}
					var plan *discoveryPlan
					var err error
					if kind == "home-values" {
						options.Locale, options.ValueKind, options.SortBy = value, value, "name"
						plan, err = homeValuesPlan(options)
					} else {
						raw, marshalErr := json.Marshal([]discoveryOptions{options})
						require.NoError(t, marshalErr)
						plan, err = discoveryPlanFor(raw, native)
					}
					require.NoError(t, err)
					require.NotContains(t, plan.statement, value)
					require.NotContains(t, plan.statement, start)
					require.NotContains(t, plan.statement, end)
					require.Contains(t, plan.args, value)
					require.Contains(t, plan.args, start)
					require.Contains(t, plan.args, end)
					if previous != nil {
						require.Equal(t, previous.statement, plan.statement)
						require.NotEqual(t, previous.args, plan.args)
					}
					previous = plan
				}
			})
		}
	}
}
