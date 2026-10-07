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
func TestAttributesThroughProductionQueryHandler(t *testing.T) {
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
	require.NoError(t, tracesExporter.ConsumeTraces(ctx, traces))
	endpoint := "http://" + address

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
