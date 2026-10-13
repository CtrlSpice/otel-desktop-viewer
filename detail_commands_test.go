package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func runDetailCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}}, http.DefaultClient, time.Now,
		func(context.Context, otelcol.CollectorSettings) error { t.Fatal("detail started a viewer"); return nil },
		func(string) error { t.Fatal("detail opened a browser"); return nil })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}

func TestLogDetailThroughProductionServer(t *testing.T) {
	endpoint, _, logsExporter, _ := startAttributeIntegration(t)
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
	require.NoError(t, logsExporter.ConsumeLogs(context.Background(), data))
	_, refs, err := requestQuery(context.Background(), http.DefaultClient, endpoint, "select id::varchar from logs order by id", 10)
	require.NoError(t, err)
	require.Len(t, refs.Rows, 2)
	var sawCorrelated, sawUncorrelated bool
	for _, row := range refs.Rows {
		ref := row[0].(string)
		want, err := requestViewerRPC(context.Background(), http.DefaultClient, endpoint, "getLog", map[string]any{"logRef": ref})
		require.NoError(t, err)
		out, err := runDetailCLI(t, "log", strings.ReplaceAll(strings.ToUpper(ref), "-", ""), "--endpoint", endpoint, "--json")
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
		out, err = runDetailCLI(t, "log", ref, "--endpoint", endpoint)
		require.NoError(t, err)
		for _, text := range []string{"LOG\n", "BODY\n", "LOG ATTRIBUTES", "RESOURCE ATTRIBUTES", "SCOPE ATTRIBUTES", "resource-schema", "scope-schema", "logger", "v1", "18446744073709551615", "9223372036854775807", "-9223372036854775808", "8000000000000000", strings.Repeat("body", 200)} {
			assert.Contains(t, out, text)
		}
		assert.NotContains(t, out, "\x1b")
	}
	assert.True(t, sawCorrelated)
	assert.True(t, sawUncorrelated)
}

func TestMetricDetailThroughProductionServer(t *testing.T) {
	endpoint, _, _, metricsExporter := startAttributeIntegration(t)
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
	require.NoError(t, metricsExporter.ConsumeMetrics(context.Background(), data))
	_, refs, err := requestQuery(context.Background(), http.DefaultClient, endpoint, "select id::varchar, name from metrics order by name", 10)
	require.NoError(t, err)
	require.Len(t, refs.Rows, 4)
	for _, row := range refs.Rows {
		t.Run(row[1].(string), func(t *testing.T) {
			ref := row[0].(string)
			out, err := runDetailCLI(t, "metric", ref, "--endpoint", endpoint, "--json")
			require.NoError(t, err)
			want, err := requestViewerRPC(context.Background(), http.DefaultClient, endpoint, "getMetric", map[string]any{"metricRef": ref})
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
			table, err := runDetailCLI(t, "metric", ref, "--endpoint", endpoint)
			require.NoError(t, err)
			for _, text := range []string{"METRIC\n", "METADATA\n", "metric-resource-schema", "metric-scope-schema", "SCOPE ATTRIBUTES", "9223372036854775807", "SERIES (", "seriesRef", "computed"} {
				assert.Contains(t, table, text)
			}
			for _, series := range catalogue.Series {
				out, err := runDetailCLI(t, "metric", ref, "--series", series.SeriesRef, "--endpoint", endpoint, "--json")
				require.NoError(t, err)
				want, err := requestViewerRPC(context.Background(), http.DefaultClient, endpoint, "getMetricSeries", map[string]any{"metricRef": ref, "seriesRef": series.SeriesRef, "startTime": nil, "endTime": nil})
				require.NoError(t, err)
				assert.Equal(t, string(want)+"\n", out)
				var selected struct{ Datapoints []map[string]json.RawMessage }
				require.NoError(t, json.Unmarshal([]byte(out), &selected))
				require.NotEmpty(t, selected.Datapoints)
				for _, point := range selected.Datapoints {
					assert.Contains(t, point, "datapointRef")
					assert.NotContains(t, point, "datapointID")
				}
				table, err := runDetailCLI(t, "metric", ref, "--series", series.SeriesRef, "--endpoint", endpoint)
				require.NoError(t, err)
				assert.Contains(t, table, "datapointRef")
				assert.Contains(t, table, "DATAPOINTS (")
				if series.DatapointCount == "30" {
					assert.Len(t, selected.Datapoints, 30, "no summary limit or implicit last-hour filter")
					assert.Contains(t, table, "0x8000000000000000")
					out, err = runDetailCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "1970-01-01T00:00:00.000000002Z", "--end", "1970-01-01T00:00:00.000000003Z", "--endpoint", endpoint, "--json")
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal([]byte(out), &selected))
					require.Len(t, selected.Datapoints, 2)
					assert.Equal(t, `"2"`, string(selected.Datapoints[0]["timestamp"]))
					assert.Equal(t, `"3"`, string(selected.Datapoints[1]["timestamp"]))
				}
				if series.LastDatapointTimestamp == "18446744073709551615" {
					out, err = runDetailCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "2554-07-21T23:34:33.709551615Z", "--endpoint", endpoint, "--json")
					require.NoError(t, err)
					assert.Contains(t, out, `"intValue":"-9223372036854775808"`)
				}
				if row[1] == "Histogram" {
					assert.Contains(t, table, "18446744073709551615")
					assert.Contains(t, table, "(absent)")
					assert.Contains(t, table, "0x8000000000000000")
					assert.Contains(t, table, "0000000000000000000000000000000a")
				}
				out, err = runDetailCLI(t, "metric", ref, "--series", series.SeriesRef, "--start", "1970-01-01T00:00:00.000000050Z", "--end", "1970-01-01T00:00:00.000000050Z", "--endpoint", endpoint)
				require.NoError(t, err)
				assert.Contains(t, out, "DATAPOINTS (0;")
			}
		})
	}
	metricA, metricB := refs.Rows[0][0].(string), refs.Rows[1][0].(string)
	_, series, err := requestQuery(context.Background(), http.DefaultClient, endpoint, fmt.Sprintf("select id::varchar from metric_series where metric_id='%s'::uuid", metricA), 10)
	require.NoError(t, err)
	out, err := runDetailCLI(t, "metric", metricB, "--series", series.Rows[0][0].(string), "--endpoint", endpoint, "--json")
	require.ErrorContains(t, err, "Metric not found")
	assert.Empty(t, out, "series belonging to another Metric must not be returned")
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
				_, err := runDetailCLI(t, append([]string{command, "--endpoint", server.URL}, args...)...)
				require.Error(t, err)
			}
			_, err := runDetailCLI(t, command, "--help", "--endpoint", server.URL)
			require.NoError(t, err)
			assert.Zero(t, requests)
			out, err := runDetailCLI(t, command, ref, "--endpoint", server.URL)
			require.ErrorContains(t, err, "Record not found")
			assert.Empty(t, out)
			out, err = runDetailCLI(t, command, strings.ToUpper(ref), "--endpoint", server.URL, "--json")
			require.ErrorContains(t, err, "Record not found")
			assert.Empty(t, out)
		})
	}
	client := &http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid args contacted viewer"); return nil, nil })}
	for _, args := range [][]string{
		{ref, "--start", "2026-10-07T08:00:00Z"}, {ref, "--end", ""}, {ref, "--series", ""},
		{ref, "--series", "bad"}, {ref, "--series", ref, "--start", "bad"},
		{ref, "--series", ref, "--start", ""},
		{ref, "--series", ref, "--start", "1969-12-31T23:59:59Z"},
		{ref, "--series", ref, "--end", "2554-07-21T23:34:33.709551616Z"},
		{ref, "--series", ref, "--start", "2026-10-07T09:00:00Z", "--end", "2026-10-07T08:00:00Z"},
	} {
		cmd := newMetricCommand(client)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		require.Error(t, cmd.Execute(), "%v", args)
	}
}

func TestDetailCommandJSONPreservesWireTokensAndErrors(t *testing.T) {
	const ref = "018f0000-0000-7000-8000-000000000001"
	const raw = `{"exact":9223372036854775807,"negativeZero":-0.0,"value":1.2500,"timestamp":"18446744073709551615","nested":{"kind":"int64","value":"-9223372036854775808"}}`
	for _, command := range []string{"log", "metric"} {
		t.Run(command, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":`+raw+`}`)
			}))
			defer server.Close()
			out, err := runDetailCLI(t, command, ref, "--endpoint", server.URL, "--json")
			require.NoError(t, err)
			assert.Equal(t, raw+"\n", out)
			out, err = runDetailCLI(t, command, ref, "--endpoint", server.URL)
			require.Error(t, err, "table output must reject an incomplete detail object")
			assert.Empty(t, out)
		})
	}
	for _, command := range []string{"log", "metric"} {
		client := &http.Client{Transport: telemetryRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}
		cmd := newLogCommand(client)
		if command == "metric" {
			cmd = newMetricCommand(client)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{ref})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		require.ErrorIs(t, cmd.Execute(), context.Canceled)
	}
}

func TestDetailMissingReferencesThroughProductionServer(t *testing.T) {
	endpoint, _, _, _ := startAttributeIntegration(t)
	const ref = "018f0000-0000-7000-8000-000000000001"
	for _, args := range [][]string{{"log", ref}, {"metric", ref}, {"metric", ref, "--series", ref}} {
		for _, jsonOutput := range []bool{false, true} {
			flags := append(append([]string{}, args...), "--endpoint", endpoint)
			if jsonOutput {
				flags = append(flags, "--json")
			}
			out, err := runDetailCLI(t, flags...)
			require.ErrorContains(t, err, "not found")
			assert.Empty(t, out)
		}
	}
}
