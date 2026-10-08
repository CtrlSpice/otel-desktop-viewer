package metrics_test

import (
	"database/sql/driver"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestScalarMetricViewIncludesInactiveValueNulls(t *testing.T) {
	for _, metricCase := range []struct {
		name        string
		temporality pmetric.AggregationTemporality
	}{
		{name: "Gauge"},
		{name: "Sum.Delta", temporality: pmetric.AggregationTemporalityDelta},
		{name: "Sum.Cumulative", temporality: pmetric.AggregationTemporalityCumulative},
	} {
		for _, valueType := range []string{"Int", "Double", "Empty"} {
			t.Run(metricCase.name+"/"+valueType, func(t *testing.T) {
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				metric := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				metric.SetName("scalar.nulls")
				var points pmetric.NumberDataPointSlice
				if metricCase.name == "Gauge" {
					points = metric.SetEmptyGauge().DataPoints()
				} else {
					sum := metric.SetEmptySum()
					sum.SetAggregationTemporality(metricCase.temporality)
					sum.SetIsMonotonic(true)
					points = sum.DataPoints()
				}
				dp := points.AppendEmpty()
				dp.SetTimestamp(100)
				switch valueType {
				case "Int":
					dp.SetIntValue(math.MaxInt64)
				case "Double":
					dp.SetDoubleValue(math.Copysign(0, -1))
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error {
					return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
				}))
				view := getMetricFullByNameInRange(t, s, ctx, "scalar.nulls", store.TimeRange{})
				datapoints := metricDatapoints(view)
				require.Len(t, datapoints, 1)
				point := datapoints[0].(map[string]any)
				require.Contains(t, point, "intValue", "an inactive field must be present, not merely compare equal to nil")
				require.Contains(t, point, "doubleValue")
				assert.Equal(t, valueType, point["valueType"])
				switch valueType {
				case "Int":
					assert.Equal(t, "9223372036854775807", point["intValue"])
					assert.Nil(t, point["doubleValue"])
				case "Double":
					assert.Nil(t, point["intValue"])
					assert.Equal(t, "0x8000000000000000", point["doubleValue"])
				case "Empty":
					assert.Nil(t, point["intValue"])
					assert.Nil(t, point["doubleValue"])
				}
				assert.NotContains(t, point, "exemplarCount", "no exemplars were withheld")
				if metricCase.name != "Gauge" {
					assert.Equal(t, float64(metricCase.temporality), point["aggregationTemporalityCode"])
					assert.Equal(t, true, point["isMonotonic"])
				}
				assert.NotContains(t, point, "delta", "Delta and first Cumulative readings have no computed delta")
				if metricCase.temporality != pmetric.AggregationTemporalityCumulative {
					assert.NotContains(t, point, "isReset")
				}
			})
		}
	}
}
