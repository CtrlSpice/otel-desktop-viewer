package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestDetailSectionsIgnoreNoncanonicalFields(t *testing.T) {
	type fieldAlias struct {
		path  []string
		key   string
		value any
	}
	endpoint, _, logsExporter, metricsExporter := startAttributeIntegration(t)
	ctx := context.Background()
	logs := plog.NewLogs()
	resource := logs.ResourceLogs().AppendEmpty()
	resource.Resource().Attributes().PutStr("resource", "resource-original")
	scope := resource.ScopeLogs().AppendEmpty()
	scope.Scope().Attributes().PutStr("scope", "scope-original")
	log := scope.LogRecords().AppendEmpty()
	log.Attributes().PutStr("key", "lowercase-attribute")
	log.Attributes().PutStr("KEY", "uppercase-attribute")
	body := log.Body().SetEmptyMap()
	body.PutStr("key", "lowercase-map")
	body.PutStr("KEY", "uppercase-map")
	body.PutEmptySlice("values").AppendEmpty().SetStr("nested-original")
	require.NoError(t, logsExporter.ConsumeLogs(ctx, logs))
	metrics := pmetric.NewMetrics()
	metricResource := metrics.ResourceMetrics().AppendEmpty()
	metricResource.Resource().Attributes().PutStr("resource", "resource-original")
	metricScope := metricResource.ScopeMetrics().AppendEmpty()
	metricScope.Scope().Attributes().PutStr("scope", "scope-original")
	m := metricScope.Metrics().AppendEmpty()
	m.SetName("probe")
	m.Metadata().PutStr("metadata", "metadata-original")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.Attributes().PutStr("key", "lowercase-attribute")
	dp.Attributes().PutStr("KEY", "uppercase-attribute")
	dp.SetIntValue(42)
	exemplar := dp.Exemplars().AppendEmpty()
	exemplar.SetIntValue(42)
	exemplar.FilteredAttributes().PutStr("filtered", "filtered-original")
	require.NoError(t, metricsExporter.ConsumeMetrics(ctx, metrics))
	_, logRefs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select id::varchar from logs", 1)
	require.NoError(t, err)
	_, metricRefs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select metric_id::varchar, id::varchar from metric_series", 1)
	require.NoError(t, err)
	logRef := logRefs.Rows[0][0].(string)
	metricRef, seriesRef := metricRefs.Rows[0][0].(string), metricRefs.Rows[0][1].(string)
	for _, tc := range []struct {
		name    string
		args    []string
		method  string
		params  map[string]any
		aliases []fieldAlias
	}{
		{"log", []string{"log", logRef}, "getLog", map[string]any{"logRef": logRef}, []fieldAlias{
			{nil, "TIMESTAMP", "not-a-timestamp"},
			{nil, "BODY", nil},
			{nil, "RESOURCE", nil},
			{nil, "SCOPE", nil},
			{nil, "ATTRIBUTES", nil},
			{[]string{"resource"}, "ATTRIBUTES", nil},
			{[]string{"resource"}, "DROPPEDATTRIBUTESCOUNT", -1},
			{[]string{"scope"}, "NAME", "alias-name"},
			{[]string{"scope"}, "VERSION", "alias-version"},
			{[]string{"scope"}, "ATTRIBUTES", nil},
			{[]string{"attributes", "0"}, "KEY", "alias-key"},
			{[]string{"attributes", "0"}, "VALUE", nil},
			{[]string{"attributes", "0", "value"}, "KIND", "alias-kind"},
			{[]string{"attributes", "0", "value"}, "VALUE", "alias-value"},
			{[]string{"body"}, "KIND", "alias-kind"},
			{[]string{"body"}, "VALUE", nil},
			{[]string{"body", "value", "0"}, "KEY", "alias-key"},
			{[]string{"body", "value", "0", "value"}, "VALUE", "alias-value"},
			{[]string{"body", "value", "2", "value", "value", "0"}, "VALUE", "alias-value"},
		}},
		{"metric", []string{"metric", metricRef}, "getMetric", map[string]any{"metricRef": metricRef}, []fieldAlias{
			{nil, "METRICTYPE", "Sum"},
			{nil, "SERIES", nil},
			{nil, "METADATA", nil},
			{nil, "RESOURCE", nil},
			{nil, "SCOPE", nil},
			{[]string{"resource"}, "SCHEMAURL", "alias-url"},
			{[]string{"scope"}, "NAME", "alias-name"},
			{[]string{"metadata", "0"}, "KEY", "alias-key"},
			{[]string{"metadata", "0", "value"}, "VALUE", "alias-value"},
			{[]string{"series", "0"}, "DATAPOINTCOUNT", "wrong"},
			{[]string{"series", "0", "attributes", "0"}, "KEY", "alias-key"},
			{[]string{"series", "0", "attributes", "0", "value"}, "VALUE", "alias-value"},
		}},
		{"series", []string{"metric", metricRef, "--series", seriesRef}, "getMetricSeries", map[string]any{"metricRef": metricRef, "seriesRef": seriesRef, "startTime": nil, "endTime": nil}, []fieldAlias{
			{nil, "SERIESREF", "alias-ref"},
			{nil, "DATAPOINTS", nil},
			{nil, "ATTRIBUTES", nil},
			{[]string{"attributes", "0", "value"}, "VALUE", "alias-value"},
			{[]string{"datapoints", "0"}, "TIMESTAMP", "wrong"},
			{[]string{"datapoints", "0", "exemplars", "0"}, "TIMESTAMP", "wrong"},
			{[]string{"datapoints", "0", "exemplars", "0", "filteredAttributes", "0"}, "KEY", "alias-key"},
			{[]string{"datapoints", "0", "exemplars", "0", "filteredAttributes", "0", "value"}, "VALUE", "alias-value"},
		}},
	} {
		raw, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, tc.method, tc.params)
		require.NoError(t, err)
		want, err := runDetailCLI(t, append(tc.args, "--endpoint", endpoint)...)
		require.NoError(t, err)
		assert.Contains(t, want, "lowercase-attribute")
		assert.Contains(t, want, "uppercase-attribute")
		if tc.name == "log" {
			assert.Contains(t, want, "lowercase-map")
			assert.Contains(t, want, "uppercase-map")
		}
		for _, alias := range tc.aliases {
			for _, first := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/%s/first=%t", tc.name, strings.Join(alias.path, "/"), alias.key, first), func(t *testing.T) {
					changed := addDetailAlias(t, raw, alias.path, alias.key, alias.value, first)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, changed)
					}))
					defer server.Close()
					args := append(append([]string{}, tc.args...), "--endpoint", server.URL)
					out, err := runDetailCLI(t, args...)
					require.NoError(t, err)
					assert.Equal(t, want, out, "noncanonical fields must not change readable output")
					out, err = runDetailCLI(t, append(args, "--json")...)
					require.NoError(t, err)
					assert.Equal(t, string(changed)+"\n", out)
				})
			}
		}
	}
}

func TestMetricSectionsOnlyRenderApplicableDescriptors(t *testing.T) {
	endpoint, _, _, exporter := startAttributeIntegration(t)
	ctx := context.Background()
	data := pmetric.NewMetrics()
	metrics := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
	for _, kind := range []string{"Gauge", "Sum", "Histogram", "ExponentialHistogram"} {
		metric := metrics.AppendEmpty()
		metric.SetName(kind)
		switch kind {
		case "Gauge":
			metric.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(42)
		case "Sum":
			sum := metric.SetEmptySum()
			sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
			sum.SetIsMonotonic(true)
			sum.DataPoints().AppendEmpty().SetIntValue(42)
		case "Histogram":
			histogram := metric.SetEmptyHistogram()
			histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			histogram.DataPoints().AppendEmpty().SetCount(0)
		case "ExponentialHistogram":
			histogram := metric.SetEmptyExponentialHistogram()
			histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			histogram.DataPoints().AppendEmpty().SetCount(0)
		}
	}
	require.NoError(t, exporter.ConsumeMetrics(ctx, data))
	_, refs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select m.metric_type, m.id::varchar, s.id::varchar from metrics m join metric_series s on s.metric_id = m.id", 4)
	require.NoError(t, err)
	require.Len(t, refs.Rows, 4)
	for _, row := range refs.Rows {
		kind, ref, seriesRef := row[0].(string), row[1].(string), row[2].(string)
		for _, selected := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/selected=%t", kind, selected), func(t *testing.T) {
				method := "getMetric"
				params := map[string]any{"metricRef": ref}
				args := []string{"metric", ref}
				if selected {
					method = "getMetricSeries"
					params["seriesRef"], params["startTime"], params["endTime"] = seriesRef, nil, nil
					args = append(args, "--series", seriesRef)
				}
				raw, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, method, params)
				require.NoError(t, err)
				want, err := runDetailCLI(t, append(args, "--endpoint", endpoint)...)
				require.NoError(t, err)
				assert.Equal(t, kind != "Gauge", strings.Contains(want, "aggregationTemporalityCode"))
				assert.Equal(t, kind == "Sum", strings.Contains(want, "isMonotonic"))
				var ignored []string
				if kind == "Gauge" {
					ignored = append(ignored, "aggregationTemporalityCode")
				}
				if kind != "Sum" {
					ignored = append(ignored, "isMonotonic")
				}
				for _, field := range ignored {
					for _, unexpected := range []any{"1.5", json.Number("2147483648"), true, map[string]any{"unexpected": true}, nil} {
						var document map[string]any
						require.NoError(t, decodeExactJSON(raw, &document))
						document[field] = unexpected
						changed, err := json.Marshal(document)
						require.NoError(t, err)
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
							fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, changed)
						}))
						out, err := runDetailCLI(t, append(args, "--endpoint", server.URL)...)
						server.Close()
						require.NoError(t, err, "irrelevant descriptor %s must be ignored", field)
						assert.Equal(t, want, out)
					}
				}
				var required []string
				if kind != "Gauge" {
					required = append(required, "aggregationTemporalityCode")
				}
				if kind == "Sum" {
					required = append(required, "isMonotonic")
				}
				for _, field := range required {
					for _, invalid := range []any{json.Number("1.5"), json.Number("2147483648"), "1.5", nil} {
						var document map[string]any
						require.NoError(t, decodeExactJSON(raw, &document))
						document[field] = invalid
						changed, err := json.Marshal(document)
						require.NoError(t, err)
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
							fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, changed)
						}))
						out, err := runDetailCLI(t, append(args, "--endpoint", server.URL)...)
						server.Close()
						require.Error(t, err, "invalid applicable descriptor %s must fail", field)
						assert.Empty(t, out)
					}
				}
			})
		}
	}
}

func addDetailAlias(t *testing.T, raw json.RawMessage, path []string, key string, value any, first bool) json.RawMessage {
	t.Helper()
	if len(path) == 0 {
		name, err := json.Marshal(key)
		require.NoError(t, err)
		payload, err := json.Marshal(value)
		require.NoError(t, err)
		member := string(name) + ":" + string(payload)
		object := strings.TrimSpace(string(raw))
		require.True(t, strings.HasPrefix(object, "{") && strings.HasSuffix(object, "}"))
		if first {
			return json.RawMessage("{" + member + "," + object[1:])
		}
		return json.RawMessage(object[:len(object)-1] + "," + member + "}")
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &items))
		index, err := strconv.Atoi(path[0])
		require.NoError(t, err)
		require.Less(t, index, len(items))
		items[index] = addDetailAlias(t, items[index], path[1:], key, value, first)
		encoded, err := json.Marshal(items)
		require.NoError(t, err)
		return encoded
	}
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &object))
	require.Contains(t, object, path[0])
	object[path[0]] = addDetailAlias(t, object[path[0]], path[1:], key, value, first)
	encoded, err := json.Marshal(object)
	require.NoError(t, err)
	return encoded
}
