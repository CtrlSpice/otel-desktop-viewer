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
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestScalarCounterArithmetic(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "scalar-counter-test")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("test.counter")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	appendInt := func(series string, timestamp uint64, value int64) {
		dp := sum.DataPoints().AppendEmpty()
		dp.Attributes().PutStr("series", series)
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(value)
	}
	appendDouble := func(series string, timestamp uint64, value float64) {
		dp := sum.DataPoints().AppendEmpty()
		dp.Attributes().PutStr("series", series)
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetDoubleValue(value)
	}

	appendInt("above-2^53", 100, 1<<53)
	appendInt("above-2^53", 200, 1<<53+1)
	appendInt("max-int64", 100, math.MaxInt64-1)
	appendInt("max-int64", 200, math.MaxInt64)
	appendInt("full-signed-span", 100, math.MinInt64)
	appendInt("full-signed-span", 200, math.MaxInt64)
	appendInt("reset", 100, 100)
	appendInt("reset", 200, 7)
	appendInt("zero", 100, 0)
	appendInt("zero", 200, 0)
	appendInt("negative", 100, -5)
	appendInt("negative", 200, -4)
	appendInt("first", 100, 42)
	appendInt("tie", 100, 10)
	appendInt("tie", 100, 11)
	appendDouble("double", 100, 1.25)
	appendDouble("double", 200, 2.5)
	appendDouble("double-integral", 100, 1)
	appendDouble("double-integral", 200, 2)
	appendDouble("double-reset", 100, 100)
	appendDouble("double-reset", 200, 7)
	appendDouble("double-negative-reset", 100, -1.5)
	appendDouble("double-negative-reset", 200, -2.25)
	appendDouble("double-signed-zero", 100, math.Copysign(0, -1))
	appendDouble("double-signed-zero", 200, 0)
	appendInt("mixed-double-after-int", 100, 1<<53+1)
	appendDouble("mixed-double-after-int", 200, float64(1<<53))
	appendDouble("mixed-int-after-double", 100, float64(1<<53))
	appendInt("mixed-int-after-double", 200, 1<<53+1)
	appendInt("mixed-fraction-down", 100, 3)
	appendDouble("mixed-fraction-down", 200, 2.5)
	appendDouble("mixed-fraction-up", 100, 2.5)
	appendInt("mixed-fraction-up", 200, 3)
	appendInt("mixed-max-to-double", 100, math.MaxInt64)
	appendDouble("mixed-max-to-double", 200, 9223372036854775808.0)
	appendDouble("mixed-double-to-max", 100, 9223372036854775808.0)
	appendInt("mixed-double-to-max", 200, math.MaxInt64)
	appendInt("mixed-min-to-double", 100, math.MinInt64)
	appendDouble("mixed-min-to-double", 200, -9223372036854775808.0)
	appendDouble("mixed-double-to-min-plus-one", 100, -9223372036854775808.0)
	appendInt("mixed-double-to-min-plus-one", 200, math.MinInt64+1)
	appendInt("mixed-above-max", 100, math.MaxInt64)
	appendDouble("mixed-above-max", 200, 9223372036854777856.0)
	appendDouble("mixed-above-max-reset", 100, 9223372036854777856.0)
	appendInt("mixed-above-max-reset", 200, math.MaxInt64)
	appendDouble("mixed-below-min", 100, -9223372036854777856.0)
	appendInt("mixed-below-min", 200, math.MinInt64)
	appendInt("mixed-below-min-reset", 100, math.MinInt64)
	appendDouble("mixed-below-min-reset", 200, -9223372036854777856.0)

	empty := sum.DataPoints().AppendEmpty()
	empty.Attributes().PutStr("series", "empty-then-int")
	empty.SetTimestamp(100)
	appendInt("empty-then-int", 200, 1)
	appendInt("int-empty-int", 100, 10)
	emptyBetween := sum.DataPoints().AppendEmpty()
	emptyBetween.Attributes().PutStr("series", "int-empty-int")
	emptyBetween.SetTimestamp(200)
	appendInt("int-empty-int", 300, 15)

	nonmonotonicMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonmonotonicMetric.SetName("test.nonmonotonic")
	nonmonotonic := nonmonotonicMetric.SetEmptySum()
	nonmonotonic.SetIsMonotonic(false)
	nonmonotonic.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for i, value := range []int64{math.MaxInt64, math.MinInt64} {
		dp := nonmonotonic.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i*100))
		dp.SetIntValue(value)
	}

	nonfiniteMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonfiniteMetric.SetName("test.nonfinite")
	nonfinite := nonfiniteMetric.SetEmptySum()
	nonfinite.SetIsMonotonic(true)
	nonfinite.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for i, value := range []float64{1, math.NaN(), math.Inf(1), math.Inf(-1), 3} {
		dp := nonfinite.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i*100))
		dp.SetDoubleValue(value)
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
	}))
	raw := getMetricFullByNameInRange(t, s, ctx, "test.counter", store.BoundedTimeRange(0, 400))

	bySeries := make(map[string]map[string]any)
	for _, rawSeries := range raw["timeseries"].([]any) {
		series := rawSeries.(map[string]any)
		attribute := series["attributes"].([]any)[0].(map[string]any)
		name := attribute["value"].(map[string]any)["value"].(string)
		bySeries[name] = series
	}
	latestFor := func(name string) map[string]any {
		return bySeries[name]["datapoints"].([]any)[0].(map[string]any)
	}

	require.Equal(t, "1", latestFor("above-2^53")["delta"])
	require.Equal(t, "1", latestFor("max-int64")["delta"])
	require.Equal(t, "18446744073709551615", latestFor("full-signed-span")["delta"])
	require.Equal(t, "7", latestFor("reset")["delta"])
	require.Equal(t, true, latestFor("reset")["isReset"])
	require.Equal(t, "0", latestFor("zero")["delta"])
	require.Equal(t, "1", latestFor("negative")["delta"])
	require.Equal(t, 1.25, latestFor("double")["delta"])
	require.Equal(t, float64(1), latestFor("double-integral")["delta"])
	require.Equal(t, float64(7), latestFor("double-reset")["delta"])
	require.Equal(t, true, latestFor("double-reset")["isReset"])
	require.Equal(t, -2.25, latestFor("double-negative-reset")["delta"])
	require.Equal(t, true, latestFor("double-negative-reset")["isReset"])
	require.Equal(t, float64(0), latestFor("double-signed-zero")["delta"])
	require.Equal(t, "1", latestFor("mixed-int-after-double")["delta"])
	require.Equal(t, false, latestFor("mixed-int-after-double")["isReset"])
	require.Equal(t, "9007199254740992", latestFor("mixed-double-after-int")["delta"])
	require.Equal(t, true, latestFor("mixed-double-after-int")["isReset"])
	require.Equal(t, 2.5, latestFor("mixed-fraction-down")["delta"])
	require.Equal(t, true, latestFor("mixed-fraction-down")["isReset"])
	require.Equal(t, 0.5, latestFor("mixed-fraction-up")["delta"])
	require.Equal(t, false, latestFor("mixed-fraction-up")["isReset"])
	require.Equal(t, "1", latestFor("mixed-max-to-double")["delta"])
	require.Equal(t, false, latestFor("mixed-max-to-double")["isReset"])
	require.Equal(t, "9223372036854775807", latestFor("mixed-double-to-max")["delta"])
	require.Equal(t, true, latestFor("mixed-double-to-max")["isReset"])
	require.Equal(t, "0", latestFor("mixed-min-to-double")["delta"])
	require.Equal(t, "1", latestFor("mixed-double-to-min-plus-one")["delta"])
	require.Equal(t, "2049", latestFor("mixed-above-max")["delta"])
	require.Equal(t, "9223372036854775807", latestFor("mixed-above-max-reset")["delta"])
	require.Equal(t, "2048", latestFor("mixed-below-min")["delta"])
	require.Equal(t, "-9223372036854777856", latestFor("mixed-below-min-reset")["delta"])
	require.NotContains(t, latestFor("first"), "delta")
	require.NotContains(t, latestFor("first"), "isReset")
	require.NotContains(t, latestFor("empty-then-int"), "delta")
	require.NotContains(t, latestFor("int-empty-int"), "delta")
	require.NotContains(t, latestFor("int-empty-int"), "isReset")

	tied := bySeries["tie"]["datapoints"].([]any)
	require.Len(t, tied, 2)
	require.NotContains(t, tied[0].(map[string]any), "delta")
	require.Contains(t, tied[1].(map[string]any), "delta")

	streamID := findMetricID(t, s, ctx, "test.counter")
	withViews, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID, store.BoundedTimeRange(0, 300),
			0, nil, nil, 0, 2, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var viewsMetric map[string]any
	require.NoError(t, json.Unmarshal(withViews, &viewsMetric))
	for _, rawSeries := range viewsMetric["timeseries"].([]any) {
		series := rawSeries.(map[string]any)
		attribute := series["attributes"].([]any)[0].(map[string]any)
		if attribute["value"].(map[string]any)["value"] != "above-2^53" {
			continue
		}
		views := series["views"].([]any)
		require.NotEmpty(t, views)
		require.Positive(t, views[len(views)-1].(map[string]any)["rate"].(float64))
	}

	nonmonotonicRaw := getMetricFullByNameInRange(t, s, ctx, "test.nonmonotonic", store.BoundedTimeRange(0, 300))
	nonmonotonicDatapoints := metricDatapoints(nonmonotonicRaw)
	require.Len(t, nonmonotonicDatapoints, 2)
	latest := nonmonotonicDatapoints[0].(map[string]any)
	require.Equal(t, "-18446744073709551615", latest["delta"])
	require.Equal(t, false, latest["isReset"])

	// Non-finite observations are excluded from arithmetic, so the finite points
	// span a delta of two. The bare NaN also records the separate existing JSON
	// transport defect without claiming that this response is valid JSON.
	nonfiniteID := findMetricID(t, s, ctx, "test.nonfinite")
	nonfiniteRaw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, nonfiniteID, store.BoundedTimeRange(0, 600),
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	require.False(t, json.Valid(nonfiniteRaw))
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":NaN`)
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":Infinity`)
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":-Infinity`)
	require.Contains(t, string(nonfiniteRaw), `"delta":2.0`)
}
