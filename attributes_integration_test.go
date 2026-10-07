package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type attributeIntegrationHost struct {
	ext component.Component
}

func (h attributeIntegrationHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{component.NewID(duckdbextension.Type): h.ext}
}

// Exercise ingestion, canonical value encoding, the production HTTP query
// handler/executor, and the actual commands together, without a browser or
// a collector subprocess. Only this test's extension owns the in-memory store.
func startAttributeIntegration(t *testing.T) (string, exporter.Traces, exporter.Logs, exporter.Metrics) {
	t.Helper()
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	factory := duckdbextension.NewFactory()
	cfg := factory.CreateDefaultConfig().(*duckdbextension.Config)
	cfg.Endpoint = address
	cfg.DbMaxSize = "0"
	ext, err := factory.Create(ctx, extensiontest.NewNopSettings(duckdbextension.Type), cfg)
	require.NoError(t, err)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, ext.Shutdown(context.Background())) })

	exporterFactory := desktopexporter.NewFactory()
	exporterCfg := exporterFactory.CreateDefaultConfig().(*desktopexporter.Config)
	exporterCfg.SendingQueue = configoptional.None[exporterhelper.QueueBatchConfig]()
	tracesExporter, err := exporterFactory.CreateTraces(ctx, exporter.Settings{
		ID: component.NewID(exporterFactory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, tracesExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, tracesExporter.Shutdown(context.Background())) })
	settings := exporter.Settings{ID: component.NewID(exporterFactory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	logsExporter, err := exporterFactory.CreateLogs(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, logsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, logsExporter.Shutdown(context.Background())) })
	metricsExporter, err := exporterFactory.CreateMetrics(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, metricsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, metricsExporter.Shutdown(context.Background())) })
	return "http://" + address, tracesExporter, logsExporter, metricsExporter
}

func TestAttributesThroughProductionQueryHandler(t *testing.T) {
	endpoint, tracesExporter, _, _ := startAttributeIntegration(t)

	bits := []uint64{
		0x0000000000000000, // positive zero
		0x8000000000000000, // negative zero
		0x0000000000000001, // smallest positive subnormal
		0x7fefffffffffffff, // largest finite double
		0xffefffffffffffff, // smallest finite double
		0x7ff0000000000000, // positive infinity
		0xfff0000000000000, // negative infinity
		0x7ff8000000000001, // NaN with payload
	}
	traces := ptrace.NewTraces()
	resource := traces.ResourceSpans().AppendEmpty()
	resource.Resource().Attributes().PutStr("resource.only", "excluded")
	spans := resource.ScopeSpans().AppendEmpty().Spans()
	for i, value := range bits {
		span := spans.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{7: 1})
		span.SetStartTimestamp(8000000000000)
		span.SetEndTimestamp(8000000000001)
		span.Attributes().PutDouble("number", math.Float64frombits(value))
		span.Attributes().PutInt("integer", math.MaxInt64)
	}
	require.NoError(t, tracesExporter.ConsumeTraces(context.Background(), traces))

	var keys attributeKeysResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, "keys", "--json")), &keys))
	assert.Equal(t, []attributeKey{
		{Key: "integer", Kind: "int64", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
		{Key: "number", Kind: "double", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
	}, keys.Keys)
	assert.False(t, keys.Truncated)

	var values attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, "values", "number", "--json", "--limit", "8")), &values))
	require.Len(t, values.Values, len(bits))
	assert.False(t, values.Truncated)
	var receivedBits []uint64
	for _, value := range values.Values {
		assert.Equal(t, uint64(1), value.Count)
		assert.Equal(t, uint64(len(bits)), value.Denominator)
		assert.Equal(t, 0.125, value.RelativeFrequency)
		assert.Equal(t, []attributeLocation{{Signal: "traces", OwnerType: "span"}}, value.FoundOn)
		var tagged struct {
			Kind  string          `json:"kind"`
			Value json.RawMessage `json:"value"`
		}
		require.NoError(t, json.Unmarshal(value.Value, &tagged))
		assert.Equal(t, "double", tagged.Kind)
		if len(tagged.Value) > 0 && tagged.Value[0] == '"' {
			var hex string
			require.NoError(t, json.Unmarshal(tagged.Value, &hex))
			var exact uint64
			_, err := fmt.Sscanf(hex, "0x%016x", &exact)
			require.NoError(t, err)
			receivedBits = append(receivedBits, exact)
		} else {
			var number float64
			require.NoError(t, json.Unmarshal(tagged.Value, &number))
			receivedBits = append(receivedBits, math.Float64bits(number))
		}
	}
	assert.ElementsMatch(t, bits, receivedBits)

	var limited attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, "values", "number", "--json", "--limit", "1")), &limited))
	require.Len(t, limited.Values, 1)
	assert.True(t, limited.Truncated)
	assert.Equal(t, uint64(8), limited.Values[0].Denominator)
	var integer attributeValuesResult
	require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, "values", "integer", "--json")), &integer))
	require.Len(t, integer.Values, 1)
	assert.JSONEq(t, `{"kind":"int64","value":"9223372036854775807"}`, string(integer.Values[0].Value))
	assert.Equal(t, uint64(8), integer.Values[0].Count)
	assert.Equal(t, 1.0, integer.Values[0].RelativeFrequency)
	assert.Contains(t, attributeTestRun(t, endpoint, "values", "integer"), "100%")
	assert.Contains(t, attributeTestRun(t, endpoint, "values", "integer"), "9223372036854775807")
}

func TestAttributeResourceScopeFrequenciesAcrossSignals(t *testing.T) {
	endpoint, te, le, me := startAttributeIntegration(t)
	traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
	// West records share one resource and scope. East uses a different owner.
	// Missing-key, out-of-window and other-service records must not change the
	// denominator. The timestamp and histogram count deliberately differ.
	groups := []struct {
		value     string
		service   string
		count     int
		timestamp pcommon.Timestamp
	}{
		{"west", "checkout", 100, 8000000000000},
		{"east", "checkout", 1, 8000000000000},
		{"", "checkout", 3, 8000000000000},
		{"west", "checkout", 7, 10000000000001},
		{"west", "other", 5, 8000000000000},
	}
	for groupIndex, group := range groups {
		rs := traces.ResourceSpans().AppendEmpty()
		rl := logs.ResourceLogs().AppendEmpty()
		rm := metrics.ResourceMetrics().AppendEmpty()
		ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
		for _, resource := range []pcommon.Resource{rs.Resource(), rl.Resource(), rm.Resource()} {
			resource.Attributes().PutStr("service.name", group.service)
			if group.value != "" {
				resource.Attributes().PutStr("region", group.value)
			}
		}
		for _, scope := range []pcommon.InstrumentationScope{ss.Scope(), sl.Scope(), sm.Scope()} {
			scope.SetName("test-scope")
			if group.value != "" {
				scope.Attributes().PutStr("region", group.value)
			}
		}
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("requests")
		histogram := metric.SetEmptyHistogram()
		histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for i := 0; i < group.count; i++ {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID{14: byte(groupIndex + 1), 15: byte(i + 1)})
			span.SetSpanID(pcommon.SpanID{7: 1})
			span.SetStartTimestamp(group.timestamp)
			span.SetEndTimestamp(group.timestamp + 1)
			log := sl.LogRecords().AppendEmpty()
			// No trace/span correlation. Alternate received and observed fallback.
			if i%2 == 0 {
				log.SetTimestamp(group.timestamp)
			} else {
				log.SetObservedTimestamp(group.timestamp)
			}
			dp := histogram.DataPoints().AppendEmpty()
			dp.SetTimestamp(group.timestamp)
			dp.SetStartTimestamp(1)
			dp.SetCount(9999)
			if group.value != "" {
				for _, attributes := range []pcommon.Map{span.Attributes(), log.Attributes(), dp.Attributes()} {
					attributes.PutStr("region", group.value)
					attributes.PutInt("exact", math.MaxInt64)
				}
			}
		}
	}
	require.NoError(t, te.ConsumeTraces(context.Background(), traces))
	require.NoError(t, le.ConsumeLogs(context.Background(), logs))
	require.NoError(t, me.ConsumeMetrics(context.Background(), metrics))
	for _, pair := range []attributeLocation{
		{Signal: "traces", OwnerType: "span"}, {Signal: "logs", OwnerType: "log"}, {Signal: "metrics", OwnerType: "datapoint"},
		{Signal: "traces", OwnerType: "resource"}, {Signal: "logs", OwnerType: "resource"}, {Signal: "metrics", OwnerType: "resource"},
		{Signal: "traces", OwnerType: "scope"}, {Signal: "logs", OwnerType: "scope"}, {Signal: "metrics", OwnerType: "scope"},
	} {
		t.Run(pair.Signal+"/"+pair.OwnerType, func(t *testing.T) {
			flags := []string{"--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "checkout", "--json"}
			var result attributeValuesResult
			require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, append([]string{"values", "region", "--limit", "2"}, flags...)...)), &result))
			require.Len(t, result.Values, 2)
			assert.False(t, result.Truncated)
			assert.Equal(t, uint64(100), result.Values[0].Count)
			assert.Equal(t, uint64(1), result.Values[1].Count)
			assert.JSONEq(t, `{"kind":"string","value":"west"}`, string(result.Values[0].Value))
			assert.JSONEq(t, `{"kind":"string","value":"east"}`, string(result.Values[1].Value))
			for _, value := range result.Values {
				assert.Equal(t, uint64(101), value.Denominator)
				assert.Equal(t, float64(value.Count)/101, value.RelativeFrequency)
				assert.Equal(t, []attributeLocation{pair}, value.FoundOn)
			}
			var limited attributeValuesResult
			require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, append([]string{"values", "region", "--limit", "1"}, flags...)...)), &limited))
			require.Len(t, limited.Values, 1)
			assert.True(t, limited.Truncated)
			assert.Equal(t, uint64(101), limited.Values[0].Denominator)
			var keys attributeKeysResult
			require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, append([]string{"keys"}, flags...)...)), &keys))
			assert.Contains(t, keys.Keys, attributeKey{Key: "region", Kind: "string", FoundOn: []attributeLocation{pair}})
			// Scope is shared across services, but counts still follow filtered records.
			var other attributeValuesResult
			require.NoError(t, json.Unmarshal([]byte(attributeTestRun(t, endpoint, "values", "region", "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "other", "--json")), &other))
			require.Len(t, other.Values, 1)
			assert.Equal(t, uint64(5), other.Values[0].Count)
			assert.Equal(t, uint64(5), other.Values[0].Denominator)
		})
	}
}
