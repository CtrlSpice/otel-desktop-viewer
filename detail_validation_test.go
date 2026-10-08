package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestDetailSectionsRejectMalformedStoreResponses(t *testing.T) {
	endpoint, _, logsExporter, metricsExporter := startAttributeIntegration(t)
	ctx := context.Background()
	logs := plog.NewLogs()
	log := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	log.Body().SetEmptyMap().PutEmptySlice("values").AppendEmpty().SetInt(math.MaxInt64)
	log.Attributes().PutStr("a", "v")
	require.NoError(t, logsExporter.ConsumeLogs(ctx, logs))
	metrics := pmetric.NewMetrics()
	m := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("probe")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetIntValue(math.MaxInt64)
	dp.Exemplars().AppendEmpty().SetDoubleValue(math.Copysign(0, -1))
	require.NoError(t, metricsExporter.ConsumeMetrics(ctx, metrics))
	_, logRefs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select id::varchar from logs", 1)
	require.NoError(t, err)
	_, metricRefs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select metric_id::varchar, id::varchar from metric_series", 1)
	require.NoError(t, err)
	logRef := logRefs.Rows[0][0].(string)
	metricRef, seriesRef := metricRefs.Rows[0][0].(string), metricRefs.Rows[0][1].(string)
	for _, tc := range []struct {
		name      string
		args      []string
		method    string
		params    map[string]any
		mutations map[string]func(map[string]any)
	}{
		{"log", []string{"log", logRef}, "getLog", map[string]any{"logRef": logRef}, map[string]func(map[string]any){
			"missing timestamp":         func(d map[string]any) { delete(d, "timestamp") },
			"missing traceID":           func(d map[string]any) { delete(d, "traceID") },
			"null body":                 func(d map[string]any) { d["body"] = nil },
			"empty body object":         func(d map[string]any) { d["body"] = map[string]any{} },
			"wrong nested typed value":  func(d map[string]any) { d["body"] = map[string]any{"kind": "array", "value": []any{nil}} },
			"null timestamp":            func(d map[string]any) { d["timestamp"] = nil },
			"wrong timestamp type":      func(d map[string]any) { d["timestamp"] = map[string]any{"unexpected": true} },
			"signed timestamp":          func(d map[string]any) { d["timestamp"] = "-1" },
			"overflow timestamp":        func(d map[string]any) { d["timestamp"] = "18446744073709551616" },
			"missing resource":          func(d map[string]any) { delete(d, "resource") },
			"missing nested attributes": func(d map[string]any) { delete(d["resource"].(map[string]any), "attributes") },
			"null scope":                func(d map[string]any) { d["scope"] = nil },
			"null severity":             func(d map[string]any) { d["severityNumber"] = nil },
			"quoted severity":           func(d map[string]any) { d["severityNumber"] = "17" },
			"missing attribute key":     func(d map[string]any) { delete(d["attributes"].([]any)[0].(map[string]any), "key") },
		}},
		{"metric", []string{"metric", metricRef}, "getMetric", map[string]any{"metricRef": metricRef}, map[string]func(map[string]any){
			"missing resource":              func(d map[string]any) { delete(d, "resource") },
			"missing scope":                 func(d map[string]any) { delete(d, "scope") },
			"missing unit":                  func(d map[string]any) { delete(d, "unit") },
			"null metadata":                 func(d map[string]any) { d["metadata"] = nil },
			"wrong series count":            func(d map[string]any) { d["series"].([]any)[0].(map[string]any)["datapointCount"] = "wrong" },
			"missing nullable summary time": func(d map[string]any) { delete(d["series"].([]any)[0].(map[string]any), "firstDatapointTimestamp") },
			"null series entry":             func(d map[string]any) { d["series"] = []any{nil} },
		}},
		{"series", []string{"metric", metricRef, "--series", seriesRef}, "getMetricSeries", map[string]any{"metricRef": metricRef, "seriesRef": seriesRef, "startTime": nil, "endTime": nil}, map[string]func(map[string]any){
			"null reference": func(d map[string]any) { d["datapoints"].([]any)[0].(map[string]any)["datapointRef"] = nil },
			"object timestamp": func(d map[string]any) {
				d["datapoints"].([]any)[0].(map[string]any)["timestamp"] = map[string]any{"unexpected": true}
			},
			"missing inactive arm": func(d map[string]any) { delete(d["datapoints"].([]any)[0].(map[string]any), "doubleValue") },
			"null active arm":      func(d map[string]any) { d["datapoints"].([]any)[0].(map[string]any)["intValue"] = nil },
			"overflow integer": func(d map[string]any) {
				d["datapoints"].([]any)[0].(map[string]any)["intValue"] = "9223372036854775808"
			},
			"wrong value kind": func(d map[string]any) { d["datapoints"].([]any)[0].(map[string]any)["valueType"] = "Bogus" },
			"null exemplars":   func(d map[string]any) { d["datapoints"].([]any)[0].(map[string]any)["exemplars"] = nil },
			"bad exemplar": func(d map[string]any) {
				d["datapoints"].([]any)[0].(map[string]any)["exemplars"] = []any{map[string]any{"timestamp": "0"}}
			},
		}},
	} {
		raw, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, tc.method, tc.params)
		require.NoError(t, err)
		for name, mutate := range tc.mutations {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				var document map[string]any
				require.NoError(t, decodeExactJSON(raw, &document))
				mutate(document)
				changed, err := json.Marshal(document)
				require.NoError(t, err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, changed)
				}))
				defer server.Close()
				args := append(append([]string{}, tc.args...), "--endpoint", server.URL)
				out, err := runDetailCLI(t, args...)
				require.Error(t, err)
				assert.Empty(t, out, "validation must precede section output")
				out, err = runDetailCLI(t, append(args, "--json")...)
				require.NoError(t, err)
				assert.Equal(t, string(changed)+"\n", out, "raw JSON passthrough remains available")
			})
		}
	}
}

func TestDetailValidationPreservesLegitimateEmptyAndSpecialValues(t *testing.T) {
	endpoint, _, logsExporter, metricsExporter := startAttributeIntegration(t)
	ctx := context.Background()
	logs := plog.NewLogs()
	log := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	// An empty received body is a tagged empty value, not a null body object.
	log.Attributes().PutEmpty("empty")
	log.Attributes().PutEmptyBytes("bytes")
	log.Attributes().PutEmptyMap("map")
	log.Attributes().PutEmptySlice("array")
	log.Attributes().PutDouble("nan", math.Float64frombits(0x7ff8000000000001))
	log.Attributes().PutDouble("infinity", math.Inf(1))
	log.Attributes().PutDouble("finite", 1.5)
	log.SetSeverityNumber(-1)
	require.NoError(t, logsExporter.ConsumeLogs(ctx, logs))
	_, refs, err := requestQuery(ctx, http.DefaultClient, endpoint, "select id::varchar from logs", 1)
	require.NoError(t, err)
	out, err := runDetailCLI(t, "log", refs.Rows[0][0].(string), "--endpoint", endpoint)
	require.NoError(t, err)
	assert.Contains(t, out, "empty")
	assert.Contains(t, out, "0x7ff8000000000001")
	assert.Contains(t, out, "0x7ff0000000000000")

	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("specials")
	points := m.SetEmptyGauge().DataPoints()
	for _, value := range []float64{math.Inf(1), math.Float64frombits(0x7ff8000000000001), 1.5} {
		dp := points.AppendEmpty()
		dp.SetDoubleValue(value)
	}
	require.NoError(t, metricsExporter.ConsumeMetrics(ctx, data))
	_, refs, err = requestQuery(ctx, http.DefaultClient, endpoint, "select metric_id::varchar, id::varchar from metric_series", 1)
	require.NoError(t, err)
	metricRef, seriesRef := refs.Rows[0][0].(string), refs.Rows[0][1].(string)
	out, err = runDetailCLI(t, "metric", metricRef, "--series", seriesRef, "--endpoint", endpoint)
	require.NoError(t, err)
	assert.Contains(t, out, "0x7ff8000000000001")
	assert.Contains(t, out, "0x7ff0000000000000")
	assert.Contains(t, out, "1.5")

	// An empty catalogue entry retains explicit null computed bounds.
	raw, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, "getMetric", map[string]any{"metricRef": metricRef})
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, decodeExactJSON(raw, &document))
	series := document["series"].([]any)[0].(map[string]any)
	series["datapointCount"] = "0"
	series["firstDatapointTimestamp"], series["lastDatapointTimestamp"] = nil, nil
	raw, err = json.Marshal(document)
	require.NoError(t, err)
	_, err = formatMetricDetail(raw, false)
	require.NoError(t, err)
}
