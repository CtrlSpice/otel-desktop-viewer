package metrics_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestAggregationTemporalityNumericIdentityAndWireProjection(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	temporalities := []pmetric.AggregationTemporality{
		pmetric.AggregationTemporalityUnspecified,
		pmetric.AggregationTemporalityDelta,
		pmetric.AggregationTemporalityCumulative,
		pmetric.AggregationTemporality(99),
		pmetric.AggregationTemporality(-1),
		pmetric.AggregationTemporality(math.MinInt32),
		pmetric.AggregationTemporality(math.MaxInt32),
	}
	labels := map[int32]string{
		0: "Unspecified", 1: "Delta", 2: "Cumulative", 99: "Unknown (99)",
		-1: "Unknown (-1)", math.MinInt32: "Unknown (-2147483648)", math.MaxInt32: "Unknown (2147483647)",
	}

	data := pmetric.NewMetrics()
	sm := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
	for i, temporality := range temporalities {
		metric := sm.AppendEmpty()
		metric.SetName("same.stream")
		metric.SetUnit("1")
		sum := metric.SetEmptySum()
		sum.SetAggregationTemporality(temporality)
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(i + 1))
		dp.SetIntValue(int64(i))
	}
	gaugeMetric := sm.AppendEmpty()
	gaugeMetric.SetName("same.stream")
	gaugeMetric.SetUnit("1")
	gaugeDP := gaugeMetric.SetEmptyGauge().DataPoints().AppendEmpty()
	gaugeDP.SetTimestamp(9)
	gaugeDP.SetIntValue(9)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	var count, distinctCodes int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `select count(*), count(distinct aggregation_temporality)
			from metric_streams where name = 'same.stream'`).Scan(&count, &distinctCodes)
	}))
	assert.Equal(t, len(temporalities)+1, count)
	assert.Equal(t, len(temporalities), distinctCodes)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, 10), nil)
	})
	require.NoError(t, err)
	var summaries []struct {
		ID                         string          `json:"id"`
		MetricType                 string          `json:"metricType"`
		AggregationTemporalityCode json.RawMessage `json:"aggregationTemporalityCode"`
		AggregationTemporality     string          `json:"aggregationTemporality"`
	}
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Len(t, summaries, len(temporalities)+1, "metric type and unknown codes must remain distinct stream identities")
	for _, summary := range summaries {
		if summary.MetricType == "Gauge" {
			assert.JSONEq(t, "null", string(summary.AggregationTemporalityCode))
			assert.Empty(t, summary.AggregationTemporality, "Gauge zero is non-applicable, not Unspecified")
		} else {
			var code int32
			require.NoError(t, json.Unmarshal(summary.AggregationTemporalityCode, &code))
			assert.Equal(t, labels[code], summary.AggregationTemporality)
		}
		detailRaw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, summary.ID, store.BoundedTimeRange(0, 10), 100, nil, nil, 0, 100, 100, nil, "UTC", nil, 100)
		})
		require.NoError(t, err)
		var detail struct {
			AggregationTemporalityCode json.RawMessage `json:"aggregationTemporalityCode"`
			AggregationTemporality     string          `json:"aggregationTemporality"`
		}
		require.NoError(t, json.Unmarshal(detailRaw, &detail))
		assert.JSONEq(t, string(summary.AggregationTemporalityCode), string(detail.AggregationTemporalityCode))
		assert.Equal(t, summary.AggregationTemporality, detail.AggregationTemporality)
	}
}
