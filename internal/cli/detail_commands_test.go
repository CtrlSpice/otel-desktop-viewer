package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func queryAllRows(t *testing.T, endpoint, sql string) [][]any {
	t.Helper()
	_, result, err := requestQuery(context.Background(), http.DefaultClient, endpoint, sql, 100)
	require.NoError(t, err)
	return result.Rows
}

func rowIDSet(rows [][]any, col int) map[string]bool {
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row[col].(string)] = true
	}
	return seen
}

func filterNewRows(rows [][]any, col int, seen map[string]bool) [][]any {
	var fresh [][]any
	for _, row := range rows {
		if !seen[row[col].(string)] {
			fresh = append(fresh, row)
		}
	}
	return fresh
}

func TestDetailThroughProductionServer(t *testing.T) {
	t.Parallel()
	endpoint, _, logsExporter, metricsExporter := startAttributeIntegration(t)
	ctx := context.Background()

	t.Run("missing references", func(t *testing.T) {
		const ref = "018f0000-0000-7000-8000-000000000001"
		for _, args := range [][]string{{"log", ref}, {"metric", ref}, {"metric", ref, "--series", ref}} {
			for _, jsonOutput := range []bool{false, true} {
				flags := append(append([]string{}, args...), "--endpoint", endpoint)
				if jsonOutput {
					flags = append(flags, "--json")
				}
				out, err := runCLI(t, flags...)
				require.ErrorContains(t, err, "not found")
				assert.Empty(t, out)
			}
		}
	})

	t.Run("log detail", func(t *testing.T) {
		seenLogs := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from logs"), 0)
		data := plog.NewLogs()
		r := data.ResourceLogs().AppendEmpty()
		r.SetSchemaUrl("resource-schema")
		r.Resource().Attributes().PutStr("service.name", "logs-service")
		r.Resource().SetDroppedAttributesCount(3)
		s := r.ScopeLogs().AppendEmpty()
		s.SetSchemaUrl("scope-schema")
		s.Scope().SetName("logger")
		s.Scope().SetVersion("v1")
		s.Scope().Attributes().PutBool("enabled", false)
		s.Scope().SetDroppedAttributesCount(4)
		for i := 0; i < 2; i++ {
			l := s.LogRecords().AppendEmpty()
			l.SetTimestamp(0)
			l.SetObservedTimestamp(pcommon.Timestamp(math.MaxUint64))
			l.SetSeverityNumber(plog.SeverityNumberError)
			l.SetSeverityText("ERROR\x1b[31m")
			l.SetEventName("complete-body")
			l.SetFlags(255)
			l.SetDroppedAttributesCount(5)
			l.Attributes().PutInt("exact", math.MaxInt64)
			body := l.Body().SetEmptyMap()
			body.PutInt("large", math.MinInt64)
			body.PutDouble("zero", math.Copysign(0, -1))
			body.PutStr("long", strings.Repeat("body", 200))
			if i == 0 {
				l.SetTraceID(pcommon.TraceID{15: 42})
				l.SetSpanID(pcommon.SpanID{7: 43})
			}
		}
		require.NoError(t, logsExporter.ConsumeLogs(ctx, data))
		refs := filterNewRows(queryAllRows(t, endpoint, "select id::varchar from logs order by id"), 0, seenLogs)
		require.Len(t, refs, 2)

		var sawCorrelated, sawUncorrelated bool
		for _, row := range refs {
			ref := row[0].(string)
			want, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, "getLog", map[string]any{"logRef": ref})
			require.NoError(t, err)
			out, err := runCLI(t, "log", strings.ReplaceAll(strings.ToUpper(ref), "-", ""), "--endpoint", endpoint, "--json")
			require.NoError(t, err)
			assert.Equal(t, string(want)+"\n", out)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			if result["traceID"] == nil {
				sawUncorrelated = true
				assert.Nil(t, result["spanID"])
			} else {
				sawCorrelated = true
				assert.Equal(t, "0000000000000000000000000000002a", result["traceID"])
				assert.Equal(t, "000000000000002b", result["spanID"])
			}
			out, err = runCLI(t, "log", ref, "--endpoint", endpoint)
			require.NoError(t, err)
			for _, text := range []string{"LOG\n", "BODY\n", "LOG ATTRIBUTES", "RESOURCE ATTRIBUTES", "SCOPE ATTRIBUTES", "resource-schema", "scope-schema", "logger", "v1", "18446744073709551615", "9223372036854775807", "-9223372036854775808", "8000000000000000", strings.Repeat("body", 200)} {
				assert.Contains(t, out, text)
			}
			assert.NotContains(t, out, "\x1b")
		}
		assert.True(t, sawCorrelated)
		assert.True(t, sawUncorrelated)
	})

	t.Run("metric detail", func(t *testing.T) {
		seenMetrics := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from metrics"), 0)
		data := pmetric.NewMetrics()
		r := data.ResourceMetrics().AppendEmpty()
		r.SetSchemaUrl("metric-resource-schema")
		r.Resource().Attributes().PutStr("service.name", "metric-service")
		r.Resource().SetDroppedAttributesCount(2)
		s := r.ScopeMetrics().AppendEmpty()
		s.SetSchemaUrl("metric-scope-schema")
		s.Scope().SetName("meter")
		s.Scope().SetVersion("v2")
		s.Scope().Attributes().PutInt("exact", math.MaxInt64)
		s.Scope().SetDroppedAttributesCount(3)
		for _, kind := range []string{"Gauge", "Sum", "Histogram", "ExponentialHistogram"} {
			m := s.Metrics().AppendEmpty()
			m.SetName(kind)
			m.SetDescription("received description")
			m.SetUnit("s")
			m.Metadata().PutStr("hint", "value")
			switch kind {
			case "Gauge", "Sum":
				var points pmetric.NumberDataPointSlice
				if kind == "Gauge" {
					points = m.SetEmptyGauge().DataPoints()
				} else {
					sum := m.SetEmptySum()
					sum.SetIsMonotonic(false)
					sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
					points = sum.DataPoints()
				}
				for i := 0; i < 30; i++ {
					dp := points.AppendEmpty()
					dp.SetTimestamp(pcommon.Timestamp(i + 1))
					dp.SetStartTimestamp(0)
					dp.Attributes().PutStr("route", "/checkout")
					switch i % 3 {
					case 0:
						dp.SetIntValue(math.MaxInt64)
					case 1:
						dp.SetDoubleValue(math.Copysign(0, -1))
					}
				}
				dp := points.AppendEmpty()
				dp.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
				dp.SetIntValue(math.MinInt64)
				dp.Attributes().PutStr("route", "/other")
			case "Histogram":
				h := m.SetEmptyHistogram()
				h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				dp := h.DataPoints().AppendEmpty()
				dp.SetTimestamp(5)
				dp.SetCount(math.MaxUint64)
				dp.BucketCounts().FromRaw([]uint64{math.MaxUint64})
				dp.SetMax(math.Copysign(0, -1))
				e := dp.Exemplars().AppendEmpty()
				e.SetTimestamp(5)
				e.SetIntValue(math.MaxInt64)
				e.SetTraceID(pcommon.TraceID{15: 10})
				e.SetSpanID(pcommon.SpanID{7: 11})
				e.FilteredAttributes().PutStr("sample", "yes")
			case "ExponentialHistogram":
				h := m.SetEmptyExponentialHistogram()
				h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				dp := h.DataPoints().AppendEmpty()
				dp.SetTimestamp(5)
				dp.SetCount(math.MaxUint64)
				dp.SetZeroCount(math.MaxUint64)
				dp.SetZeroThreshold(math.Copysign(0, -1))
				dp.SetScale(-2)
				dp.SetSum(0)
			}
		}
		require.NoError(t, metricsExporter.ConsumeMetrics(ctx, data))
		refs := filterNewRows(queryAllRows(t, endpoint, "select id::varchar, name from metrics order by name"), 0, seenMetrics)
		require.Len(t, refs, 4)

		for _, row := range refs {
			t.Run(row[1].(string), func(t *testing.T) {
				ref := row[0].(string)
				out, err := runCLI(t, "metric", ref, "--endpoint", endpoint, "--json")
				require.NoError(t, err)
				want, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, "getMetric", map[string]any{"metricRef": ref})
				require.NoError(t, err)
				assert.Equal(t, string(want)+"\n", out)
				var catalogue struct {
					Series []struct {
						SeriesRef              string
						DatapointCount         string
						LastDatapointTimestamp string
					}
				}
				require.NoError(t, json.Unmarshal([]byte(out), &catalogue))
				require.NotEmpty(t, catalogue.Series)
				table, err := runCLI(t, "metric", ref, "--endpoint", endpoint)
				require.NoError(t, err)
				for _, text := range []string{"METRIC\n", "METADATA\n", "metric-resource-schema", "metric-scope-schema", "SCOPE ATTRIBUTES", "9223372036854775807", "SERIES (", "seriesRef", "computed"} {
					assert.Contains(t, table, text)
				}
				for _, series := range catalogue.Series {
					out, err := runCLI(t, "metric", ref, "--series", series.SeriesRef, "--endpoint", endpoint, "--json")
					require.NoError(t, err)
					want, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, "getMetricSeries", map[string]any{"metricRef": ref, "seriesRef": series.SeriesRef, "startTime": nil, "endTime": nil})
					require.NoError(t, err)
					assert.Equal(t, string(want)+"\n", out)
					var selected struct{ Datapoints []map[string]json.RawMessage }
					require.NoError(t, json.Unmarshal([]byte(out), &selected))
					require.NotEmpty(t, selected.Datapoints)
					for _, point := range selected.Datapoints {
						assert.Contains(t, point, "datapointRef")
						assert.NotContains(t, point, "datapointID")
					}
					table, err := runCLI(t, "metric", ref, "--series", series.SeriesRef, "--endpoint", endpoint)
					require.NoError(t, err)
					assert.Contains(t, table, "datapointRef")
					assert.Contains(t, table, "DATAPOINTS (")
					if series.DatapointCount == "30" {
						assert.Len(t, selected.Datapoints, 30, "no summary limit or implicit last-hour filter")
						assert.Contains(t, table, "0x8000000000000000")
						out, err = runCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "1970-01-01T00:00:00.000000002Z", "--end", "1970-01-01T00:00:00.000000003Z", "--endpoint", endpoint, "--json")
						require.NoError(t, err)
						require.NoError(t, json.Unmarshal([]byte(out), &selected))
						require.Len(t, selected.Datapoints, 2)
						assert.Equal(t, `"2"`, string(selected.Datapoints[0]["timestamp"]))
						assert.Equal(t, `"3"`, string(selected.Datapoints[1]["timestamp"]))
					}
					if series.LastDatapointTimestamp == "18446744073709551615" {
						out, err = runCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "2554-07-21T23:34:33.709551615Z", "--endpoint", endpoint, "--json")
						require.NoError(t, err)
						assert.Contains(t, out, `"intValue":"-9223372036854775808"`)
					}
					if row[1] == "Histogram" {
						assert.Contains(t, table, "18446744073709551615")
						assert.Contains(t, table, "(absent)")
						assert.Contains(t, table, "0x8000000000000000")
						assert.Contains(t, table, "0000000000000000000000000000000a")
					}
					out, err = runCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "1970-01-01T00:00:00.000000050Z", "--end", "1970-01-01T00:00:00.000000050Z", "--endpoint", endpoint)
					require.NoError(t, err)
					assert.Contains(t, out, "DATAPOINTS (0;")
				}
			})
		}
		metricA, metricB := refs[0][0].(string), refs[1][0].(string)
		seriesRows := queryAllRows(t, endpoint, fmt.Sprintf("select id::varchar from metric_series where metric_id='%s'::uuid", metricA))
		require.NotEmpty(t, seriesRows)
		out, err := runCLI(t, "metric", metricB, "--series", seriesRows[0][0].(string), "--endpoint", endpoint, "--json")
		require.ErrorContains(t, err, "Metric not found")
		assert.Empty(t, out, "series belonging to another Metric must not be returned")
	})

	t.Run("sections ignore noncanonical fields", func(t *testing.T) {
		type fieldAlias struct {
			path  []string
			key   string
			value any
		}
		seenLogs := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from logs"), 0)
		seenSeries := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from metric_series"), 0)

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

		logRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar from logs"), 0, seenLogs)
		require.Len(t, logRows, 1)
		seriesRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar, metric_id::varchar from metric_series"), 0, seenSeries)
		require.Len(t, seriesRows, 1)
		logRef := logRows[0][0].(string)
		seriesRef, metricRef := seriesRows[0][0].(string), seriesRows[0][1].(string)

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
			want, err := runCLI(t, append(tc.args, "--endpoint", endpoint)...)
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
						client := staticRPCClient(changed)
						args := append(append([]string{}, tc.args...), "--endpoint", "http://viewer.test")
						out, _, err := execCLI(t, cliOptions{client: client}, args...)
						require.NoError(t, err)
						assert.Equal(t, want, out, "noncanonical fields must not change readable output")
						out, _, err = execCLI(t, cliOptions{client: client}, append(args, "--json")...)
						require.NoError(t, err)
						assert.Equal(t, string(changed)+"\n", out)
					})
				}
			}
		}
	})

	t.Run("metric sections only render applicable descriptors", func(t *testing.T) {
		seenSeries := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from metric_series"), 0)
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
		require.NoError(t, metricsExporter.ConsumeMetrics(ctx, data))
		refs := filterNewRows(queryAllRows(t, endpoint, "select s.id::varchar, m.metric_type, m.id::varchar from metrics m join metric_series s on s.metric_id = m.id"), 0, seenSeries)
		require.Len(t, refs, 4)

		for _, row := range refs {
			seriesRef, kind, ref := row[0].(string), row[1].(string), row[2].(string)
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
					want, err := runCLI(t, append(args, "--endpoint", endpoint)...)
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
							out, _, err := execCLI(t, cliOptions{client: staticRPCClient(changed)}, append(args, "--endpoint", "http://viewer.test")...)
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
							out, _, err := execCLI(t, cliOptions{client: staticRPCClient(changed)}, append(args, "--endpoint", "http://viewer.test")...)
							require.Error(t, err, "invalid applicable descriptor %s must fail", field)
							assert.Empty(t, out)
						}
					}
				})
			}
		}
	})

	t.Run("sections reject malformed store responses", func(t *testing.T) {
		seenLogs := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from logs"), 0)
		seenSeries := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from metric_series"), 0)

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

		logRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar from logs"), 0, seenLogs)
		require.Len(t, logRows, 1)
		seriesRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar, metric_id::varchar from metric_series"), 0, seenSeries)
		require.Len(t, seriesRows, 1)
		logRef := logRows[0][0].(string)
		seriesRef, metricRef := seriesRows[0][0].(string), seriesRows[0][1].(string)

		for _, tc := range []struct {
			name      string
			args      []string
			method    string
			params    map[string]any
			mutations map[string]func(map[string]any)
		}{
			{"log", []string{"log", logRef}, "getLog", map[string]any{"logRef": logRef}, map[string]func(map[string]any){
				"missing timestamp": func(d map[string]any) { delete(d, "timestamp") },
				"uppercase-only timestamp": func(d map[string]any) {
					d["TIMESTAMP"] = d["timestamp"]
					delete(d, "timestamp")
				},
				"uppercase-only body value": func(d map[string]any) {
					body := d["body"].(map[string]any)
					body["VALUE"] = body["value"]
					delete(body, "value")
				},
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
				"uppercase-only series": func(d map[string]any) {
					d["SERIES"] = d["series"]
					delete(d, "series")
				},
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
					client := staticRPCClient(changed)
					args := append(append([]string{}, tc.args...), "--endpoint", "http://viewer.test")
					out, _, err := execCLI(t, cliOptions{client: client}, args...)
					require.Error(t, err)
					assert.Empty(t, out, "validation must precede section output")
					out, _, err = execCLI(t, cliOptions{client: client}, append(args, "--json")...)
					require.NoError(t, err)
					assert.Equal(t, string(changed)+"\n", out, "raw JSON passthrough remains available")
				})
			}
		}
	})

	t.Run("validation preserves legitimate empty and special values", func(t *testing.T) {
		seenLogs := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from logs"), 0)
		seenSeries := rowIDSet(queryAllRows(t, endpoint, "select id::varchar from metric_series"), 0)

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

		logRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar from logs"), 0, seenLogs)
		require.Len(t, logRows, 1)
		out, err := runCLI(t, "log", logRows[0][0].(string), "--endpoint", endpoint)
		require.NoError(t, err)
		assert.Contains(t, out, "empty")
		assert.Contains(t, out, "0x7ff8000000000000")
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

		seriesRows := filterNewRows(queryAllRows(t, endpoint, "select id::varchar, metric_id::varchar from metric_series"), 0, seenSeries)
		require.Len(t, seriesRows, 1)
		seriesRef, metricRef := seriesRows[0][0].(string), seriesRows[0][1].(string)
		out, err = runCLI(t, "metric", metricRef, "--series", seriesRef, "--endpoint", endpoint)
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
	})
}

func TestDetailCommandRequestsValidationAndMissingRecords(t *testing.T) {
	const ref = "018f0000-0000-7000-8000-000000000001"
	for _, command := range []string{"log", "metric"} {
		t.Run(command, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var req queryRPCRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				assert.Equal(t, "get"+strings.ToUpper(command[:1])+command[1:], req.Method)
				assert.Equal(t, map[string]any{command + "Ref": ref}, req.Params)
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32003,"message":"Record not found"}}`)
			}))
			defer server.Close()
			for _, args := range [][]string{{}, {"bad"}, {ref, "extra"}, {ref, "--limit", "1"}} {
				_, err := runCLI(t, append([]string{command, "--endpoint", server.URL}, args...)...)
				require.Error(t, err)
			}
			_, err := runCLI(t, command, "--help", "--endpoint", server.URL)
			require.NoError(t, err)
			assert.Zero(t, requests)
			out, err := runCLI(t, command, ref, "--endpoint", server.URL)
			require.ErrorContains(t, err, "Record not found")
			assert.Empty(t, out)
			out, err = runCLI(t, command, strings.ToUpper(ref), "--endpoint", server.URL, "--json")
			require.ErrorContains(t, err, "Record not found")
			assert.Empty(t, out)
		})
	}
	client := offlineHTTPClient(t)
	for _, args := range [][]string{
		{ref, "--start", "2026-10-07T08:00:00Z"}, {ref, "--end", ""}, {ref, "--series", ""},
		{ref, "--series", "bad"}, {ref, "--series", ref, "--start", "bad"},
		{ref, "--series", ref, "--start", ""},
		{ref, "--series", ref, "--start", "1969-12-31T23:59:59Z"},
		{ref, "--series", ref, "--end", "2554-07-21T23:34:33.709551616Z"},
		{ref, "--series", ref, "--start", "2026-10-07T09:00:00Z", "--end", "2026-10-07T08:00:00Z"},
	} {
		_, _, err := execCLI(t, cliOptions{client: client}, append([]string{"metric"}, args...)...)
		require.Error(t, err, "%v", args)
	}
}

func TestDetailCommandJSONPreservesWireTokensAndErrors(t *testing.T) {
	const ref = "018f0000-0000-7000-8000-000000000001"
	const raw = `{"exact":9223372036854775807,"negativeZero":-0.0,"value":1.2500,"timestamp":"18446744073709551615","nested":{"kind":"int64","value":"-9223372036854775808"}}`
	for _, command := range []string{"log", "metric"} {
		t.Run(command, func(t *testing.T) {
			server := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+raw+`}`)
			out, err := runCLI(t, command, ref, "--endpoint", server.URL, "--json")
			require.NoError(t, err)
			assert.Equal(t, raw+"\n", out)
			out, err = runCLI(t, command, ref, "--endpoint", server.URL)
			require.Error(t, err, "table output must reject an incomplete detail object")
			assert.Empty(t, out)
		})
	}
	for _, command := range []string{"log", "metric"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := execCLI(t, cliOptions{client: canceledHTTPClient(), ctx: ctx}, command, ref)
		require.ErrorIs(t, err, context.Canceled)
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
