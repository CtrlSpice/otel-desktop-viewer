package metrics_test

import (
	"database/sql"
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

func TestGetMetricOTLPProtobuf(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, otlpMetricFixture(), s.FlushedIDs())
	}))
	for name, id := range metricMetricIDs(t, s, ctx) {
		t.Run(name, func(t *testing.T) {
			wire, err := readStore(s, func(db *sql.DB) ([]byte, error) {
				return metrics.GetMetricOTLPProtobuf(ctx, db, id)
			})
			require.NoError(t, err)
			decoded, err := (&pmetric.ProtoUnmarshaler{}).UnmarshalMetrics(wire)
			require.NoError(t, err)
			jsonDecoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(getMetricOTLP(t, s, ctx, id))
			require.NoError(t, err)
			// Compare every retained field. Check sign bits separately below because
			// pdata's JSON marshaler itself omits a negative-zero zeroThreshold.
			expected, err := (&pmetric.JSONMarshaler{}).MarshalMetrics(jsonDecoded)
			require.NoError(t, err)
			actual, err := (&pmetric.JSONMarshaler{}).MarshalMetrics(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(actual))
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
				return metrics.Ingest(targetCtx, conn, decoded, target.FlushedIDs())
			}))
			replayed := getMetricOTLP(t, target, targetCtx, metricMetricIDs(t, target, targetCtx)[name])
			assert.JSONEq(t, string(getMetricOTLP(t, s, ctx, id)), string(replayed))
			if metric.Type() == pmetric.MetricTypeExponentialHistogram {
				assert.Contains(t, string(replayed), `"zeroThreshold":-0.0`)
			}
		})
	}
}
