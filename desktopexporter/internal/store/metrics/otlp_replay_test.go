package metrics_test

import (
	"database/sql/driver"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestGetMetricOTLPJSONReplay(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, otlpMetricFixture())
	}))
	for name, id := range metricMetricIDs(t, s, ctx) {
		t.Run(name, func(t *testing.T) {
			decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(getMetricOTLP(t, s, ctx, id))
			require.NoError(t, err)
			metric := decoded.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
			switch metric.Type() {
			case pmetric.MetricTypeGauge:
				for _, dp := range metric.Gauge().DataPoints().All() {
					if dp.Timestamp() == 2 {
						assert.Equal(t, uint64(1<<63), math.Float64bits(dp.DoubleValue()))
					}
					if dp.Timestamp() == pcommon.Timestamp(math.MaxUint64) {
						assert.Equal(t, int64(math.MinInt64), dp.IntValue())
						exemplar := dp.Exemplars().At(0)
						assert.Equal(t, pcommon.TraceID(mustDecodeTraceIDMetrics("11111111111111112222222222222222")), exemplar.TraceID())
						assert.Equal(t, pcommon.SpanID(mustDecodeSpanIDMetrics("8000000000000000")), exemplar.SpanID())
					}
				}
			case pmetric.MetricTypeHistogram:
				dp := metric.Histogram().DataPoints().At(0)
				assert.False(t, dp.HasSum())
				assert.True(t, dp.HasMin())
				assert.Equal(t, uint64(1<<63), math.Float64bits(dp.Min()))
			case pmetric.MetricTypeExponentialHistogram:
				dp := metric.ExponentialHistogram().DataPoints().At(0)
				assert.Equal(t, uint64(1<<63), math.Float64bits(dp.ZeroThreshold()))
				assert.True(t, dp.HasSum())
				assert.Equal(t, uint64(0), math.Float64bits(dp.Sum()))
				assert.True(t, math.IsInf(dp.Min(), -1))
				assert.True(t, math.IsNaN(dp.Max()))
			}
			// A second real store must retain the same negative zero after replay.
			target, targetCtx := storetest.New(t)
			require.NoError(t, target.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(targetCtx, conn, decoded)
			}))
			replayed := getMetricOTLP(t, target, targetCtx, metricMetricIDs(t, target, targetCtx)[name])
			assert.JSONEq(t, string(getMetricOTLP(t, s, ctx, id)), string(replayed))
			if metric.Type() == pmetric.MetricTypeExponentialHistogram {
				assert.Contains(t, string(replayed), `"zeroThreshold":-0.0`)
			}
		})
	}
}

func TestGetMetricOTLPMetadataSignedZeroReplay(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "scalar", true: "nested"}[nested], func(t *testing.T) {
			s, ctx := storetest.New(t)
			data := pmetric.NewMetrics()
			ms := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
			for i, value := range []float64{0, math.Copysign(0, -1)} {
				m := ms.AppendEmpty()
				m.SetName("signed-zero-metadata")
				attrs := m.Metadata()
				if nested {
					attrs = attrs.PutEmptySlice("nested").AppendEmpty().SetEmptyMap()
				}
				attrs.PutDouble("zero", value)
				dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
				dp.SetTimestamp(pcommon.Timestamp(i + 1))
				dp.SetIntValue(int64(i))
			}
			// Check actual pdata JSON replay and a second store, not JSON equality
			// (whose numeric comparison treats both signs as equal).
			for range 2 {
				require.NoError(t, s.WithConn(func(c driver.Conn) error { return metrics.Ingest(ctx, c, data) }))
				id := metricMetricIDs(t, s, ctx)["signed-zero-metadata"]
				decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(getMetricOTLP(t, s, ctx, id))
				require.NoError(t, err)
				got := decoded.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
				require.Equal(t, 2, got.Len())
				for _, m := range got.All() {
					attrs := m.Metadata()
					if nested {
						attrs = mustMapValue(t, attrs, "nested").Slice().At(0).Map()
					}
					dp := m.Gauge().DataPoints()
					require.Equal(t, 1, dp.Len())
					require.Equal(t, dp.At(0).IntValue() == 1, math.Signbit(mustMapValue(t, attrs, "zero").Double()))
				}
				data = decoded
				s, ctx = storetest.New(t)
			}
		})
	}
}
