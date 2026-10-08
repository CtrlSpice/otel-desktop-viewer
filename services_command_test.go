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
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func servicesTestRun(t *testing.T, endpoint string, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}},
		http.DefaultClient, func() time.Time { return time.Unix(10000, 0) },
		func(context.Context, otelcol.CollectorSettings) error {
			t.Fatal("services started a viewer")
			return nil
		},
		func(string) error { t.Fatal("services opened a browser"); return nil })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(append([]string{"services", "--endpoint", endpoint}, args...))
	err := cmd.Execute()
	return output.String(), err
}

func servicesTestResult(t *testing.T, endpoint string, args ...string) servicesResult {
	t.Helper()
	output, err := servicesTestRun(t, endpoint, append(args, "--json")...)
	require.NoError(t, err)
	var result servicesResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	return result
}

func TestServicesCountsAcrossSignalsAndNamespaces(t *testing.T) {
	endpoint, te, le, me := startAttributeIntegration(t)
	traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
	for group, namespace := range []string{"shop", "shop", "internal"} {
		rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
		for _, resource := range []pcommon.Resource{rs.Resource(), rl.Resource(), rm.Resource()} {
			resource.Attributes().PutStr("service.name", "payments")
			resource.Attributes().PutStr("service.namespace", namespace)
			resource.Attributes().PutStr("service.instance.id", fmt.Sprintf("instance-%d", group))
			resource.Attributes().PutStr("service.version", fmt.Sprintf("v%d", group))
		}
		ss := rs.ScopeSpans().AppendEmpty()
		for i, code := range []ptrace.StatusCode{ptrace.StatusCodeError, ptrace.StatusCodeUnset} {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID{14: byte(group + 1), 15: byte(i + 1)})
			span.SetSpanID(pcommon.SpanID{7: 1}) // Same span ID in different traces counts separately.
			span.SetStartTimestamp(8000000000000)
			span.SetEndTimestamp(8000000000001)
			span.Status().SetCode(code)
			span.Attributes().PutStr("service.namespace", "not-the-resource")
		}
		sl := rl.ScopeLogs().AppendEmpty()
		for _, severity := range []plog.SeverityNumber{plog.SeverityNumberError, plog.SeverityNumberFatal4, plog.SeverityNumberWarn} {
			log := sl.LogRecords().AppendEmpty()
			log.SetTimestamp(8500000000000)
			log.SetSeverityNumber(severity)
			// No correlation IDs: service discovery must still count these logs.
		}
		for scopeIndex := range 2 {
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName(fmt.Sprintf("scope-%d", scopeIndex))
			metric := sm.Metrics().AppendEmpty()
			metric.SetName("work") // Same name, distinct Metric identities by Scope.
			gauge := metric.SetEmptyGauge()
			for i := range 2 {
				dp := gauge.DataPoints().AppendEmpty()
				dp.SetTimestamp(9000000000000 + pcommon.Timestamp(i))
				dp.SetIntValue(int64(i))
				dp.Attributes().PutStr("route", fmt.Sprintf("/%d", i))
			}
		}
		histMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
		histMetric.SetName("duration")
		hist := histMetric.SetEmptyHistogram()
		hist.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		dp := hist.DataPoints().AppendEmpty()
		dp.SetTimestamp(9000000000002)
		dp.SetCount(1000)
		dp.BucketCounts().FromRaw([]uint64{1000})
	}
	logOnly := logs.ResourceLogs().AppendEmpty()
	logOnly.Resource().Attributes().PutStr("service.name", "logs-only")
	logOnly.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetObservedTimestamp(8500000000001)
	metricOnly := metrics.ResourceMetrics().AppendEmpty()
	metricOnly.Resource().Attributes().PutStr("service.name", "metrics-only")
	metric := metricOnly.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("usage")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(8500000000002)
	dp.SetDoubleValue(1)
	stale := logs.ResourceLogs().AppendEmpty()
	stale.Resource().Attributes().PutStr("service.name", "outside-window")
	stale.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(10000000000001)
	empty := metrics.ResourceMetrics().AppendEmpty()
	empty.Resource().Attributes().PutStr("service.name", "no-datapoints")
	emptyMetric := empty.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	emptyMetric.SetName("empty")
	emptyMetric.SetEmptyGauge()
	require.NoError(t, te.ConsumeTraces(context.Background(), traces))
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	require.NoError(t, me.ConsumeMetrics(context.Background(), metrics))

	result := servicesTestResult(t, endpoint)
	require.NotNil(t, result.StartTime)
	require.NotNil(t, result.EndTime)
	assert.Equal(t, "6400000000000", *result.StartTime)
	assert.Equal(t, "10000000000000", *result.EndTime)
	assert.False(t, result.Truncated)
	assert.Equal(t, []serviceSummary{
		{ServiceName: "logs-only", LogCount: 1, LastSeen: "8500000000001"},
		{ServiceName: "metrics-only", MetricCount: 1, DataPointCount: 1, LastSeen: "8500000000002"},
		{ServiceNamespace: "internal", ServiceName: "payments", SpanCount: 2, ErrorSpanCount: 1, LogCount: 3, ErrorLogCount: 2, MetricCount: 3, DataPointCount: 5, LastSeen: "9000000000002"},
		{ServiceNamespace: "shop", ServiceName: "payments", SpanCount: 4, ErrorSpanCount: 2, LogCount: 6, ErrorLogCount: 4, MetricCount: 6, DataPointCount: 10, LastSeen: "9000000000002"},
	}, result.Services)
	filtered := servicesTestResult(t, endpoint, "--service", "payments")
	assert.Equal(t, result.Services[2:], filtered.Services, "name-only filtering must retain separate namespaces")
	assert.Empty(t, servicesTestResult(t, endpoint, "--service", "pay").Services, "the existing service filter is exact, not substring matching")
	limited := servicesTestResult(t, endpoint, "--limit", "3")
	assert.True(t, limited.Truncated)
	assert.Equal(t, result.Services[:3], limited.Services)
	assert.False(t, servicesTestResult(t, endpoint, "--limit", "4").Truncated)
	output, err := servicesTestRun(t, endpoint, "--limit", "3")
	require.NoError(t, err)
	for _, field := range []string{"serviceNamespace", "serviceName", "spanCount", "errorSpanCount", "logCount", "errorLogCount", "metricCount", "dataPointCount", "lastSeen", "more rows available"} {
		assert.Contains(t, output, field)
	}
}

func TestServicesNamespacePresenceAndQuotedNames(t *testing.T) {
	endpoint, _, le, _ := startAttributeIntegration(t)
	logs := plog.NewLogs()
	name := "quote' OR true --\n\x1b[31m"
	for i, namespace := range []string{"", "", "shop", "internal"} {
		rl := logs.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", name)
		if i != 0 {
			rl.Resource().Attributes().PutStr("service.namespace", namespace)
		}
		log := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		log.SetTimestamp(8000000000000 + pcommon.Timestamp(i))
	}
	unnamed := logs.ResourceLogs().AppendEmpty()
	unnamed.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(8000000000004)
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	result := servicesTestResult(t, endpoint, "--service", name)
	assert.Equal(t, []serviceSummary{
		{ServiceName: name, LogCount: 2, LastSeen: "8000000000001"},
		{ServiceNamespace: "internal", ServiceName: name, LogCount: 1, LastSeen: "8000000000003"},
		{ServiceNamespace: "shop", ServiceName: name, LogCount: 1, LastSeen: "8000000000002"},
	}, result.Services)
	all := servicesTestResult(t, endpoint)
	require.Len(t, all.Services, 4)
	assert.Equal(t, serviceSummary{LogCount: 1, LastSeen: "8000000000004"}, all.Services[0])
	output, err := servicesTestRun(t, endpoint, "--service", name)
	require.NoError(t, err)
	assert.NotContains(t, output, "\x1b")
	assert.Contains(t, output, `\n\u001b[31m`)

	// Summary grouping must not erase namespace presence in the received Resource.
	values := attributeTestRun(t, endpoint, "values", "service.namespace", "--signal", "logs", "--owner-type", "resource", "--json")
	var attributes attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(values), &attributes))
	require.Len(t, attributes.Values, 3)
	for _, value := range attributes.Values {
		assert.Equal(t, uint64(1), value.Count)
		assert.Equal(t, uint64(3), value.Denominator)
	}
}

func TestServicesTimeBoundsSeverityAndExactTimestamps(t *testing.T) {
	endpoint, te, le, me := startAttributeIntegration(t)
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "time-test")
	sl := rl.ScopeLogs().AppendEmpty()
	for _, record := range []struct {
		timestamp, observed uint64
		severity            plog.SeverityNumber
	}{
		{6400000000000, 0, 17}, {10000000000000, 0, 24}, {0, 8000000000000, 16},
		{8000000000001, 10000000000001, 0}, {8000000000002, 0, 25}, {8000000000003, 0, -1},
		{6399999999999, 8000000000000, 17}, {10000000000001, 8000000000000, 17},
		{0, 0, 0}, {math.MaxUint64, 0, 17},
	} {
		log := sl.LogRecords().AppendEmpty()
		log.SetTimestamp(pcommon.Timestamp(record.timestamp))
		log.SetObservedTimestamp(pcommon.Timestamp(record.observed))
		log.SetSeverityNumber(record.severity)
		log.SetSeverityText("ERROR") // Text must not override received numeric severity.
	}
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "time-test")
	ss := rs.ScopeSpans().AppendEmpty()
	for i, timestamp := range []uint64{6400000000000, 10000000000000, 6399999999999, 10000000000001} {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{7: 1})
		span.SetStartTimestamp(pcommon.Timestamp(timestamp))
		span.SetEndTimestamp(pcommon.Timestamp(timestamp + 5000000000000))
		span.Status().SetCode(ptrace.StatusCode(i + 1)) // Only received code 2 counts as Error.
	}
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "time-test")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("measurement")
	gauge := metric.SetEmptyGauge()
	for _, timestamp := range []uint64{6400000000000, 10000000000000, 6399999999999, 10000000000001} {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(1)
	}
	require.NoError(t, te.ConsumeTraces(context.Background(), traces))
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	require.NoError(t, me.ConsumeMetrics(context.Background(), metrics))
	assert.Equal(t, []serviceSummary{{ServiceName: "time-test", SpanCount: 2, ErrorSpanCount: 1, LogCount: 6, ErrorLogCount: 2, MetricCount: 1, DataPointCount: 2, LastSeen: "10000000000000"}}, servicesTestResult(t, endpoint).Services)
	maxTime := "2554-07-21T23:34:33.709551615Z"
	assert.Equal(t, []serviceSummary{{ServiceName: "time-test", LogCount: 1, ErrorLogCount: 1, LastSeen: "18446744073709551615"}}, servicesTestResult(t, endpoint, "--start", maxTime, "--end", maxTime).Services)
	openEnd := servicesTestResult(t, endpoint, "--start", maxTime)
	assert.Nil(t, openEnd.EndTime)
	assert.Equal(t, "18446744073709551615", openEnd.Services[0].LastSeen)
	zero := servicesTestResult(t, endpoint, "--end", "1970-01-01T00:00:00Z")
	assert.Nil(t, zero.StartTime)
	assert.Equal(t, []serviceSummary{{ServiceName: "time-test", LogCount: 1, LastSeen: "0"}}, zero.Services)
}

func TestServicesDefaultLimitAndEmptyStore(t *testing.T) {
	endpoint, _, le, _ := startAttributeIntegration(t)
	output, err := servicesTestRun(t, endpoint, "--json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"startTime":"6400000000000","endTime":"10000000000000","services":[],"truncated":false}`, output)
	logs := plog.NewLogs()
	for i := range 26 {
		rl := logs.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", fmt.Sprintf("service-%02d", i))
		rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(8000000000000)
	}
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	result := servicesTestResult(t, endpoint)
	require.Len(t, result.Services, 25)
	assert.True(t, result.Truncated)
	assert.Equal(t, "service-00", result.Services[0].ServiceName)
	assert.Equal(t, "service-24", result.Services[24].ServiceName)
	full := servicesTestResult(t, endpoint, "--limit", "26")
	require.Len(t, full.Services, 26)
	assert.False(t, full.Truncated)
}

func TestServicesDoNotCoerceNonStringIdentity(t *testing.T) {
	for _, key := range []string{"service.name", "service.namespace"} {
		t.Run(key, func(t *testing.T) {
			endpoint, _, le, _ := startAttributeIntegration(t)
			logs := plog.NewLogs()
			for i := range 2 {
				rl := logs.ResourceLogs().AppendEmpty()
				rl.Resource().Attributes().PutStr("service.name", "42")
				rl.Resource().Attributes().PutStr("service.namespace", "42")
				if i == 1 {
					rl.Resource().Attributes().PutInt(key, 42)
				}
				rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(8000000000000 + pcommon.Timestamp(i))
			}
			require.NoError(t, le.ConsumeLogs(context.Background(), logs))
			output, err := servicesTestRun(t, endpoint, "--json")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Resource "+key+" must be a string")
			assert.Empty(t, output, "a failed query must not emit a partial summary")
			// A malformed resource outside the chosen window must not block valid data.
			valid := servicesTestResult(t, endpoint, "--end", "1970-01-01T02:13:20Z")
			assert.Equal(t, []serviceSummary{{ServiceName: "42", ServiceNamespace: "42", LogCount: 1, LastSeen: "8000000000000"}}, valid.Services)
		})
	}
}

func TestServicesOfflineHelpAndInvalidArguments(t *testing.T) {
	client := &http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("help or invalid arguments made an HTTP request")
		return nil, nil
	})}
	cmd := newRootCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}}, client, time.Now,
		func(context.Context, otelcol.CollectorSettings) error { t.Fatal("help started a viewer"); return nil },
		func(string) error { t.Fatal("help opened a browser"); return nil })
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetErr(&help)
	cmd.SetArgs([]string{"services", "--help"})
	require.NoError(t, cmd.Execute())
	for _, expected := range []string{"otel-desktop-viewer services", "service.namespace", "--endpoint", "--service", "--since", "--start", "--end", "--limit", "--json"} {
		assert.Contains(t, help.String(), expected)
	}
	for _, args := range [][]string{
		{"unexpected"}, {"--limit", "0"}, {"--limit", "-1"}, {"--limit", "9223372036854775807"},
		{"--since", "0s"}, {"--since", "1h", "--start", "2026-10-02T00:00:00Z"},
		{"--start", "bad"}, {"--start", "2026-10-03T00:00:00Z", "--end", "2026-10-02T00:00:00Z"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newServicesCommand(client, time.Now)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
		})
	}
}

func TestServicesTransportPreservesLargeCounts(t *testing.T) {
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"columns":[{"name":"entry","type":"JSON"}],"rows":[[{"serviceNamespace":"shop","serviceName":"api","spanCount":9007199254740993,"errorSpanCount":0,"logCount":0,"errorLogCount":0,"metricCount":0,"dataPointCount":18446744073709551615,"lastSeen":"18446744073709551615"}]],"truncated":false}}`)
	}))
	defer viewer.Close()
	result := servicesTestResult(t, viewer.URL)
	require.Len(t, result.Services, 1)
	assert.Equal(t, uint64(9007199254740993), result.Services[0].SpanCount)
	assert.Equal(t, uint64(math.MaxUint64), result.Services[0].DataPointCount)
	output, err := servicesTestRun(t, viewer.URL)
	require.NoError(t, err)
	assert.Contains(t, output, "9007199254740993")
	assert.Contains(t, output, "18446744073709551615")
}
