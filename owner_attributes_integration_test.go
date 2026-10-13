package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Decode repeated wire entries rather than constructing a Go map, which would
// remove duplicate keys before ingestion ever sees them.
func decodedOwnerAttributes(t *testing.T, raw string) pcommon.Map {
	t.Helper()
	logs, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(
		`{"resourceLogs":[{"resource":{"attributes":` + raw + `}}]}`))
	require.NoError(t, err)
	return logs.ResourceLogs().At(0).Resource().Attributes()
}

func TestLastAttributeSelectionIsLocalToEveryOwner(t *testing.T) {
	endpoint, te, le, me := startAttributeIntegration(t)
	traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
	rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
	ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{15: 1})
	span.SetSpanID(pcommon.SpanID{7: 1})
	span.SetStartTimestamp(8000000000000)
	span.SetEndTimestamp(8000000000001)
	event := span.Events().AppendEmpty()
	event.SetTimestamp(8000000000000)
	link := span.Links().AppendEmpty()
	log := sl.LogRecords().AppendEmpty()
	log.SetTimestamp(8000000000000)
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("usage")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(8000000000000)
	dp.SetIntValue(1)
	exemplar := dp.Exemplars().AppendEmpty()
	exemplar.SetTimestamp(8000000000000)
	exemplar.SetIntValue(1)
	owners := []struct {
		location attributeLocation
		attrs    pcommon.Map
	}{
		{attributeLocation{"traces", "resource"}, rs.Resource().Attributes()},
		{attributeLocation{"traces", "scope"}, ss.Scope().Attributes()},
		{attributeLocation{"traces", "span"}, span.Attributes()},
		{attributeLocation{"traces", "event"}, event.Attributes()},
		{attributeLocation{"traces", "link"}, link.Attributes()},
		{attributeLocation{"logs", "resource"}, rl.Resource().Attributes()},
		{attributeLocation{"logs", "scope"}, sl.Scope().Attributes()},
		{attributeLocation{"logs", "log"}, log.Attributes()},
		{attributeLocation{"metrics", "resource"}, rm.Resource().Attributes()},
		{attributeLocation{"metrics", "scope"}, sm.Scope().Attributes()},
		{attributeLocation{"metrics", "metadata"}, metric.Metadata()},
		{attributeLocation{"metrics", "datapoint"}, dp.Attributes()},
		{attributeLocation{"metrics", "exemplar"}, exemplar.FilteredAttributes()},
	}
	for i, owner := range owners {
		decodedOwnerAttributes(t, fmt.Sprintf(`[
{"key":"shared","value":{"stringValue":"discard"}},
{"key":"shared","value":{"intValue":"%d"}},
{"key":"Shared","value":{"boolValue":true}}]`, i+1)).CopyTo(owner.attrs)
	}
	// Another span is a separate owner even within the same Resource and Scope.
	other := ss.Spans().AppendEmpty()
	other.SetTraceID(pcommon.TraceID{15: 2})
	other.SetSpanID(pcommon.SpanID{7: 1})
	other.SetStartTimestamp(8000000000000)
	other.SetEndTimestamp(8000000000001)
	decodedOwnerAttributes(t, `[{"key":"shared","value":{"intValue":"99"}},
{"key":"shared","value":{"intValue":"100"}}]`).CopyTo(other.Attributes())
	decodedBody, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"kvlistValue":{"values":[
{"key":"x","value":{"stringValue":"old"}},
{"key":"x","value":{"intValue":"9223372036854775807"}},
{"key":"array","value":{"arrayValue":{"values":[{"intValue":"2"},{"intValue":"2"}]}}}
]}}}]}]}]}`))
	require.NoError(t, err)
	decodedBody.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().CopyTo(log.Body())
	require.NoError(t, te.ConsumeTraces(context.Background(), traces))
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	require.NoError(t, me.ConsumeMetrics(context.Background(), metrics))
	for i, owner := range owners {
		var result attributeValuesResult
		pair := owner.location
		output := attributeTestRun(t, endpoint, "values", "shared", "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--json")
		require.NoError(t, json.Unmarshal([]byte(output), &result))
		want := []string{fmt.Sprintf(`{"kind":"int64","value":"%d"}`, i+1)}
		if pair.OwnerType == "span" {
			want = append(want, `{"kind":"int64","value":"100"}`)
		}
		require.Len(t, result.Values, len(want), pair)
		var actual []string
		for _, value := range result.Values {
			actual = append(actual, string(value.Value))
		}
		assert.ElementsMatch(t, want, actual, pair)
		assert.Equal(t, 3, owner.attrs.Len(), "incoming collections remain untouched")
	}
	_, refs, err := requestQuery(context.Background(), http.DefaultClient, endpoint, "SELECT id::VARCHAR FROM logs", 1)
	require.NoError(t, err)
	require.Len(t, refs.Rows, 1)
	ref, ok := refs.Rows[0][0].(string)
	require.True(t, ok)
	for _, query := range []string{"", "?format=json"} {
		response, err := http.Get(endpoint + "/export/logs/" + ref + query)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		wire, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		exported, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(wire)
		require.NoError(t, err)
		resource := exported.ResourceLogs().At(0)
		scope := resource.ScopeLogs().At(0)
		record := scope.LogRecords().At(0)
		for i, attrs := range []pcommon.Map{resource.Resource().Attributes(), scope.Scope().Attributes(), record.Attributes()} {
			require.Equal(t, 2, attrs.Len())
			value, found := attrs.Get("shared")
			require.True(t, found)
			assert.Equal(t, int64(i+6), value.Int())
		}
		require.Equal(t, 2, record.Body().Map().Len())
		winner, found := record.Body().Map().Get("x")
		require.True(t, found)
		assert.Equal(t, int64(9223372036854775807), winner.Int())
		array, found := record.Body().Map().Get("array")
		require.True(t, found)
		assert.Equal(t, 2, array.Slice().Len(), "array repetition survives export")
	}
}

func TestServiceLabelsFollowSelectedResourceAttributes(t *testing.T) {
	endpoint, te, le, me := startAttributeIntegration(t)
	traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
	rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
	attrs := decodedOwnerAttributes(t, `[
{"key":"service.name","value":{"stringValue":"discard"}},
{"key":"service.name","value":{"intValue":"42"}},
{"key":"service.namespace","value":{"stringValue":"discard"}},
{"key":"service.namespace","value":{"stringValue":"shop"}}]`)
	for _, resource := range []pcommon.Resource{rs.Resource(), rl.Resource(), rm.Resource()} {
		attrs.CopyTo(resource.Attributes())
	}
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{15: 1})
	span.SetSpanID(pcommon.SpanID{7: 1})
	span.SetStartTimestamp(8000000000000)
	span.SetEndTimestamp(8000000000001)
	rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(8000000000000)
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("usage")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(8000000000000)
	dp.SetIntValue(1)
	healthy := logs.ResourceLogs().AppendEmpty()
	healthy.Resource().Attributes().PutStr("service.name", "healthy")
	healthy.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(8000000000000)
	require.NoError(t, te.ConsumeTraces(context.Background(), traces))
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	require.NoError(t, me.ConsumeMetrics(context.Background(), metrics))
	result := servicesTestResult(t, endpoint)
	assert.Equal(t, []serviceSummary{
		{ServiceName: "healthy", LogCount: 1, LastSeen: "8000000000000"},
		{ServiceName: "42", ServiceNamespace: "shop", SpanCount: 1, LogCount: 1, MetricCount: 1, DataPointCount: 1, LastSeen: "8000000000000"},
	}, result.Services)
	assert.Equal(t, result.Services[1:], servicesTestResult(t, endpoint, "--service", "42").Services)
	assert.Empty(t, servicesTestResult(t, endpoint, "--service", "discard").Services)
}
