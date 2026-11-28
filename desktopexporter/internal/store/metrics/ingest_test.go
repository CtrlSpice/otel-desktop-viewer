package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

const maxNano = 1<<63 - 1

// These values keep each quantile fixture timestamp in a distinct bucket.
const (
	testQuantileWindowStartTs int64 = 0
	testQuantileWindowEndTs   int64 = 4_000_000_000_000_000_000 // ~year 2096 in nanoseconds
	testQuantileWindowPoints  int   = 1_000_000_000
)

func metricWireUint64(t *testing.T, value any) uint64 {
	t.Helper()
	text, ok := value.(string)
	require.True(t, ok, "metric uint64 wire value must be decimal text, got %T", value)
	parsed, err := strconv.ParseUint(text, 10, 64)
	require.NoError(t, err)
	return parsed
}

// createTestMetricsPdataN builds n gauge metrics under one resource and scope.
func createTestMetricsPdataN(n int) pmetric.Metrics {
	base := int64(1_700_000_000_000_000_000)
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-service")
	rm.Resource().Attributes().PutStr("resource.key", "resource.val")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")
	sm.Scope().Attributes().PutStr("scope.key", "scope.val")
	for i := 0; i < n; i++ {
		m := sm.Metrics().AppendEmpty()
		m.SetName("flush_metric_" + fmt.Sprintf("%d", i))
		m.SetDescription("Batch metric")
		m.SetUnit("count")
		g := m.SetEmptyGauge()
		dp := g.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base + int64(i)))
		dp.SetStartTimestamp(pcommon.Timestamp(base))
		dp.SetDoubleValue(float64(i))
	}
	return metrics
}

// createTestMetricsPdata returns pmetric.Metrics with four metrics: gauge, sum, histogram, exponential histogram.
func createTestMetricsPdata() pmetric.Metrics {
	base := time.Now().UnixNano()
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().SetDroppedAttributesCount(7)
	rm.Resource().Attributes().PutStr("service.name", "test-service")
	rm.Resource().Attributes().PutStr("service.version", "1.0.0")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")
	sm.Scope().SetDroppedAttributesCount(9)

	// Gauge
	m0 := sm.Metrics().AppendEmpty()
	m0.SetName("gauge_metric")
	m0.SetDescription("Current memory usage")
	m0.SetUnit("bytes")
	gauge := m0.SetEmptyGauge()
	dp0 := gauge.DataPoints().AppendEmpty()
	dp0.SetTimestamp(pcommon.Timestamp(base))
	dp0.SetStartTimestamp(pcommon.Timestamp(base - int64(time.Hour)))
	dp0.SetDoubleValue(1024.5)
	dp0.Attributes().PutStr("memory.type", "heap")
	ex0 := dp0.Exemplars().AppendEmpty()
	ex0.SetTimestamp(pcommon.Timestamp(base - int64(time.Minute)))
	ex0.SetDoubleValue(1000.0)
	ex0.SetTraceID(storetest.MustDecodeTraceID("00000000000000000000000000000099"))
	ex0.SetSpanID(storetest.MustDecodeSpanID("0000000000000001"))
	ex0.FilteredAttributes().PutStr("exemplar.source", "gauge")

	// Gauge with Int value (covers numberDataPointValue Int branch: return nil, dp.IntValue(), typeStr)
	m0int := sm.Metrics().AppendEmpty()
	m0int.SetName("gauge_int_metric")
	m0int.SetDescription("Integer gauge")
	m0int.SetUnit("count")
	gaugeInt := m0int.SetEmptyGauge()
	dp0int := gaugeInt.DataPoints().AppendEmpty()
	dp0int.SetTimestamp(pcommon.Timestamp(base + int64(time.Minute)))
	dp0int.SetStartTimestamp(pcommon.Timestamp(base))
	dp0int.SetIntValue(42)
	dp0int.Attributes().PutStr("type", "int")

	// Sum
	m1 := sm.Metrics().AppendEmpty()
	m1.SetName("sum_metric")
	m1.SetDescription("Total requests processed")
	m1.SetUnit("requests")
	sum := m1.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp1 := sum.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.Timestamp(base + int64(2*time.Minute)))
	dp1.SetDoubleValue(1500.0)
	ex1 := dp1.Exemplars().AppendEmpty()
	ex1.SetTimestamp(pcommon.Timestamp(base + int64(2*time.Minute)))
	ex1.SetDoubleValue(1400.0)
	ex1.SetTraceID(storetest.MustDecodeTraceID("00000000000000000000000000000099"))
	ex1.SetSpanID(storetest.MustDecodeSpanID("0000000000000002"))
	ex1.FilteredAttributes().PutStr("exemplar.source", "sum")

	// Histogram
	m2 := sm.Metrics().AppendEmpty()
	m2.SetName("histogram_metric")
	m2.SetDescription("Request duration distribution")
	m2.SetUnit("seconds")
	hist := m2.SetEmptyHistogram()
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp2 := hist.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.Timestamp(base + int64(4*time.Minute)))
	dp2.SetCount(100)
	dp2.SetSum(25.5)
	dp2.SetMin(0.1)
	dp2.SetMax(2.5)
	dp2.BucketCounts().FromRaw([]uint64{10, 20, 30, 25, 15})
	dp2.ExplicitBounds().FromRaw([]float64{0.5, 1.0, 1.5, 2.0})
	ex2 := dp2.Exemplars().AppendEmpty()
	ex2.SetTimestamp(pcommon.Timestamp(base + int64(4*time.Minute)))
	ex2.SetDoubleValue(1.25)
	ex2.SetTraceID(storetest.MustDecodeTraceID("00000000000000000000000000000099"))
	ex2.SetSpanID(storetest.MustDecodeSpanID("0000000000000007"))
	ex2.FilteredAttributes().PutStr("exemplar.source", "histogram")

	// Exponential histogram
	m3 := sm.Metrics().AppendEmpty()
	m3.SetName("exponential_histogram_metric")
	m3.SetDescription("Response size distribution")
	m3.SetUnit("bytes")
	exp := m3.SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp3 := exp.DataPoints().AppendEmpty()
	dp3.SetTimestamp(pcommon.Timestamp(base + int64(6*time.Minute)))
	dp3.SetCount(50)
	dp3.SetSum(10240.0)
	dp3.SetMin(100.0)
	dp3.SetMax(2048.0)
	dp3.SetScale(2)
	dp3.SetZeroCount(5)
	dp3.Positive().SetOffset(1)
	dp3.Positive().BucketCounts().FromRaw([]uint64{5, 10, 15, 10, 5})
	dp3.Negative().SetOffset(0)
	dp3.Negative().BucketCounts().FromRaw([]uint64{2, 3})
	ex3 := dp3.Exemplars().AppendEmpty()
	ex3.SetTimestamp(pcommon.Timestamp(base + int64(6*time.Minute)))
	ex3.SetDoubleValue(512.0)
	ex3.SetTraceID(storetest.MustDecodeTraceID("00000000000000000000000000000099"))
	ex3.SetSpanID(storetest.MustDecodeSpanID("000000000000000a"))
	ex3.FilteredAttributes().PutStr("exemplar.source", "exponential_histogram")

	return metrics
}

// searchMetricsAll returns metrics.SearchSummaries with wide time range and nil query; parses JSON to slice of maps.
func searchMetricsAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, maxNano), nil)
	})
	assert.NoError(t, err)
	var out []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func searchSummariesAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, maxNano), nil)
	})
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func findSummary(t *testing.T, summaries []map[string]any, name string) map[string]any {
	t.Helper()
	for _, s := range summaries {
		if s["name"] == name {
			return s
		}
	}
	t.Fatalf("summary %q not found", name)
	return nil
}

// getMetricFullByName resolves a Metric reference via SearchSummaries and fetches
// full MetricViewData via GetMetricView (timeseries, datapoints, resource, scope).
func getMetricFullByName(t *testing.T, s *store.Store, ctx context.Context, name string) map[string]any {
	return getMetricFullByNameInRange(t, s, ctx, name, store.BoundedTimeRange(0, maxNano))
}

func getMetricFullByNameInRange(t *testing.T, s *store.Store, ctx context.Context, name string, timeRange store.TimeRange) map[string]any {
	t.Helper()
	id := findMetricRef(t, s, ctx, name)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, id, timeRange, 0, nil, nil, 0, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func getExactMetricSeriesByName(t *testing.T, s *store.Store, ctx context.Context, name string, timeRange store.TimeRange) map[string]any {
	t.Helper()
	metricID := findMetricRef(t, s, ctx, name)
	discoveryRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, metricID)
	})
	require.NoError(t, err)
	var discovery map[string]any
	require.NoError(t, json.Unmarshal(discoveryRaw, &discovery))
	series := discovery["series"].([]any)
	require.Len(t, series, 1)
	seriesID := series[0].(map[string]any)["seriesRef"].(string)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricSeries(ctx, db, metricID, seriesID, timeRange)
	})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func exactMetricSeriesDatapoints(t *testing.T, series map[string]any) []any {
	t.Helper()
	return series["datapoints"].([]any)
}

func TestExactMetricSeriesPreservesMetricOwnership(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	for i, timestamp := range []pcommon.Timestamp{100, 200} {
		data := pmetric.NewMetrics()
		rm := data.ResourceMetrics().AppendEmpty()
		rm.SetSchemaUrl(fmt.Sprintf("resource-schema-%d", i))
		rm.Resource().SetDroppedAttributesCount(uint32(i + 1))
		nested := rm.Resource().Attributes().PutEmptyMap("nested")
		nested.PutInt("exact", math.MaxInt64)
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.SetSchemaUrl("scope-identity-schema")
		sm.Scope().SetName("sdk")
		sm.Scope().SetVersion("1")
		sm.Scope().SetDroppedAttributesCount(uint32(i + 3))
		sm.Scope().Attributes().PutEmptySlice("levels").AppendEmpty().SetInt(math.MinInt64)
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("owned.gauge")
		metric.SetDescription(fmt.Sprintf("report-%d", i))
		metric.Metadata().PutInt("batch", int64(i))
		point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		point.SetTimestamp(timestamp)
		point.Attributes().PutStr("series", "one")
		point.SetIntValue(int64(i + 1))
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, data)
		}))
	}

	metricIDs, err := storetest.Read(s, func(db *sql.DB) ([]string, error) {
		rows, err := db.QueryContext(ctx, `select m.id::varchar from metrics m
			where m.name = 'owned.gauge' order by m.resource.schema_url`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	})
	require.NoError(t, err)
	require.Len(t, metricIDs, 2)
	for i, metricID := range metricIDs {
		discoveryRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, metricID)
		})
		require.NoError(t, err)
		var discovery map[string]any
		require.NoError(t, json.Unmarshal(discoveryRaw, &discovery))
		require.Equal(t, fmt.Sprintf("report-%d", i), discovery["description"])
		require.Len(t, discovery["metadata"].([]any), 1)
		resource := discovery["resource"].(map[string]any)
		require.Equal(t, fmt.Sprintf("resource-schema-%d", i), resource["schemaUrl"])
		require.Equal(t, float64(i+1), resource["droppedAttributesCount"])
		scope := discovery["scope"].(map[string]any)
		require.Equal(t, float64(i+3), scope["droppedAttributesCount"])

		seriesSummary := discovery["series"].([]any)[0].(map[string]any)
		require.Equal(t, "1", seriesSummary["datapointCount"])
		seriesID := seriesSummary["seriesRef"].(string)
		selectedRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeries(ctx, db, metricID, seriesID, store.TimeRange{})
		})
		require.NoError(t, err)
		var selected map[string]any
		require.NoError(t, json.Unmarshal(selectedRaw, &selected))
		datapoints := selected["datapoints"].([]any)
		require.Len(t, datapoints, 1)
		require.Equal(t, fmt.Sprint((i+1)*100), datapoints[0].(map[string]any)["timestamp"])
	}
}

func TestResourceSchemaURLChangesKeepMetricIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource-schema-url.db")
	ctx := context.Background()

	appendMetric := func(batch pmetric.Metrics, resourceSchemaURL, scopeSchemaURL, description string, timestamp pcommon.Timestamp) {
		rm := batch.ResourceMetrics().AppendEmpty()
		rm.SetSchemaUrl(resourceSchemaURL)
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		rm.Resource().SetDroppedAttributesCount(2)
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.SetSchemaUrl(scopeSchemaURL)
		sm.Scope().SetName("sdk")
		sm.Scope().SetVersion("1")
		sm.Scope().SetDroppedAttributesCount(3)
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("requests")
		metric.SetDescription(description)
		metric.Metadata().PutStr("report", description)
		point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		point.SetTimestamp(timestamp)
		point.Attributes().PutStr("route", "/checkout")
		point.SetIntValue(int64(timestamp))
	}

	ingestBatch := func(t *testing.T, s *store.Store, batch pmetric.Metrics) {
		t.Helper()
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, batch)
		}))
	}

	s, err := store.NewStore(ctx, path, zap.NewNop())
	require.NoError(t, err)
	first := pmetric.NewMetrics()
	appendMetric(first, "resource-a", "scope-a", "first", 1)
	appendMetric(first, "resource-b", "scope-a", "second", 2)
	ingestBatch(t, s, first)

	second := pmetric.NewMetrics()
	appendMetric(second, "resource-c", "scope-a", "third", 3)
	ingestBatch(t, s, second)
	require.NoError(t, s.Close())

	s, err = store.NewStore(ctx, path, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	third := pmetric.NewMetrics()
	appendMetric(third, "resource-d", "scope-a", "fourth", 4)
	ingestBatch(t, s, third)

	var metricID string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var metricsCount, seriesCount, datapointCount, resourceCount, payloadCount int
		if err := db.QueryRow(`select count(*), min(id::varchar) from metrics where name = 'requests'`).Scan(&metricsCount, &metricID); err != nil {
			return err
		}
		if err := db.QueryRow(`select count(*) from metric_series`).Scan(&seriesCount); err != nil {
			return err
		}
		if err := db.QueryRow(`select count(*) from metric_datapoints`).Scan(&datapointCount); err != nil {
			return err
		}
		if err := db.QueryRow(`select count(distinct resource.id), count(distinct resource.payload_id) from metric_datapoints`).Scan(&resourceCount, &payloadCount); err != nil {
			return err
		}
		require.Equal(t, 1, metricsCount)
		require.Equal(t, 1, seriesCount)
		require.Equal(t, 4, datapointCount)
		require.Equal(t, 4, resourceCount)
		require.Equal(t, 1, payloadCount)
		return nil
	}))

	discoveryRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, metricID)
	})
	require.NoError(t, err)
	var discovery map[string]any
	require.NoError(t, json.Unmarshal(discoveryRaw, &discovery))
	assert.Equal(t, "fourth", discovery["description"])
	assert.Equal(t, "resource-d", discovery["resource"].(map[string]any)["schemaUrl"])

	otlp, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricOTLP(ctx, db, metricID)
	})
	require.NoError(t, err)
	decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(otlp)
	require.NoError(t, err)
	require.Equal(t, 4, decoded.ResourceMetrics().Len())
	seen := map[string]int64{}
	for _, rm := range decoded.ResourceMetrics().All() {
		require.Equal(t, uint32(2), rm.Resource().DroppedAttributesCount())
		require.Equal(t, 1, rm.ScopeMetrics().Len())
		sm := rm.ScopeMetrics().At(0)
		require.Equal(t, "scope-a", sm.SchemaUrl())
		require.Equal(t, uint32(3), sm.Scope().DroppedAttributesCount())
		require.Equal(t, 1, sm.Metrics().Len())
		points := sm.Metrics().At(0).Gauge().DataPoints()
		require.Equal(t, 1, points.Len())
		seen[rm.SchemaUrl()] = points.At(0).IntValue()
	}
	require.Equal(t, map[string]int64{"resource-a": 1, "resource-b": 2, "resource-c": 3, "resource-d": 4}, seen)

	differentScope := pmetric.NewMetrics()
	appendMetric(differentScope, "resource-e", "scope-b", "different scope", 5)
	ingestBatch(t, s, differentScope)
	require.Equal(t, 2, storetest.CountRows(t, s, ctx, `select count(*) from metrics where name = 'requests'`),
		"Scope schema URL remains part of Metric identity")
}

func TestReceivedMetricIntegersUseExactDecimalWireValues(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	sm := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()

	gaugeMetric := sm.Metrics().AppendEmpty()
	gaugeMetric.SetName("exact.gauge")
	gauge := gaugeMetric.SetEmptyGauge()
	for i, value := range []int64{math.MinInt64, math.MaxInt64, 1<<53 + 1, 0, -1} {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i))
		dp.SetIntValue(value)
	}
	doublePoint := gauge.DataPoints().AppendEmpty()
	doublePoint.SetTimestamp(106)
	doublePoint.SetDoubleValue(1.25)

	sumMetric := sm.Metrics().AppendEmpty()
	sumMetric.SetName("exact.sum")
	sum := sumMetric.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	sumPoint := sum.DataPoints().AppendEmpty()
	sumPoint.SetTimestamp(200)
	sumPoint.SetIntValue(math.MinInt64)

	maxUint64 := ^uint64(0)
	histogramMetric := sm.Metrics().AppendEmpty()
	histogramMetric.SetName("exact.histogram")
	histogram := histogramMetric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	histogramMax := histogram.DataPoints().AppendEmpty()
	histogramMax.SetTimestamp(300)
	histogramMax.SetCount(maxUint64)
	histogramMax.BucketCounts().FromRaw([]uint64{maxUint64})
	histogramEmpty := histogram.DataPoints().AppendEmpty()
	histogramEmpty.SetTimestamp(301)
	histogramEmpty.SetCount(0)

	exponentialMetric := sm.Metrics().AppendEmpty()
	exponentialMetric.SetName("exact.exponential")
	exponential := exponentialMetric.SetEmptyExponentialHistogram()
	exponential.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	exponentialPoint := exponential.DataPoints().AppendEmpty()
	exponentialPoint.SetTimestamp(400)
	exponentialPoint.SetCount(maxUint64)
	exponentialPoint.SetZeroCount(1<<53 + 1)
	exponentialPoint.Positive().BucketCounts().FromRaw([]uint64{maxUint64})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	fixtureRange := store.BoundedTimeRange(0, 500)
	gaugeDatapoints := metricDatapoints(getMetricFullByNameInRange(t, s, ctx, "exact.gauge", fixtureRange))
	var gaugeIntegers []string
	nullIntegerArms := 0
	for _, raw := range gaugeDatapoints {
		dp := raw.(map[string]any)
		if dp["intValue"] == nil {
			nullIntegerArms++
			continue
		}
		gaugeIntegers = append(gaugeIntegers, dp["intValue"].(string))
	}
	assert.ElementsMatch(t, []string{
		"-9223372036854775808",
		"9223372036854775807",
		"9007199254740993",
		"0",
		"-1",
	}, gaugeIntegers)
	assert.Equal(t, 1, nullIntegerArms)

	sumDatapoints := metricDatapoints(getMetricFullByNameInRange(t, s, ctx, "exact.sum", fixtureRange))
	require.Len(t, sumDatapoints, 1)
	assert.Equal(t, "-9223372036854775808", sumDatapoints[0].(map[string]any)["intValue"])

	histogramDatapoints := metricDatapoints(getMetricFullByNameInRange(t, s, ctx, "exact.histogram", fixtureRange))
	require.Len(t, histogramDatapoints, 2)
	byCount := make(map[string]map[string]any, len(histogramDatapoints))
	for _, raw := range histogramDatapoints {
		dp := raw.(map[string]any)
		byCount[dp["count"].(string)] = dp
	}
	assert.Equal(t, []any{"18446744073709551615"}, byCount["18446744073709551615"]["bucketCounts"])
	assert.Equal(t, []any{}, byCount["0"]["bucketCounts"])

	exponentialDatapoints := metricDatapoints(getMetricFullByNameInRange(t, s, ctx, "exact.exponential", fixtureRange))
	require.Len(t, exponentialDatapoints, 1)
	exponentialJSON := exponentialDatapoints[0].(map[string]any)
	assert.Equal(t, "18446744073709551615", exponentialJSON["count"])
	assert.Equal(t, "9007199254740993", exponentialJSON["zeroCount"])
	assert.Equal(t, []any{"18446744073709551615"}, exponentialJSON["positiveBucketCounts"])
	assert.Equal(t, []any{}, exponentialJSON["negativeBucketCounts"])

	receivedGauge := exactMetricSeriesDatapoints(t,
		getExactMetricSeriesByName(t, s, ctx, "exact.gauge", fixtureRange))
	require.Len(t, receivedGauge, 6)
	var receivedGaugeIntegers []string
	for _, raw := range receivedGauge {
		dp := raw.(map[string]any)
		require.Contains(t, dp, "intValue")
		require.Contains(t, dp, "doubleValue")
		if dp["valueType"] == "Int" {
			receivedGaugeIntegers = append(receivedGaugeIntegers, dp["intValue"].(string))
		}
	}
	assert.ElementsMatch(t, gaugeIntegers, receivedGaugeIntegers)

	receivedSum := exactMetricSeriesDatapoints(t,
		getExactMetricSeriesByName(t, s, ctx, "exact.sum", fixtureRange))
	require.Len(t, receivedSum, 1)
	assert.Equal(t, "-9223372036854775808", receivedSum[0].(map[string]any)["intValue"])

	receivedHistogram := exactMetricSeriesDatapoints(t,
		getExactMetricSeriesByName(t, s, ctx, "exact.histogram", fixtureRange))
	require.Len(t, receivedHistogram, 2)
	assert.Equal(t, "18446744073709551615", receivedHistogram[0].(map[string]any)["count"])

	receivedExponential := exactMetricSeriesDatapoints(t,
		getExactMetricSeriesByName(t, s, ctx, "exact.exponential", fixtureRange))
	require.Len(t, receivedExponential, 1)
	assert.Equal(t, "18446744073709551615", receivedExponential[0].(map[string]any)["count"])
	positive := receivedExponential[0].(map[string]any)["positive"].(map[string]any)
	assert.Equal(t, []any{"18446744073709551615"}, positive["bucketCounts"])
}

func TestExemplarSpanIDsRoundTripAndEmptyIsNull(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "native-span-ids")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("exemplar_span_ids")
	gauge := m.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetTimestamp(100)
	dp.SetDoubleValue(1)
	traceID := storetest.MustDecodeTraceID("000000000000000000000000000000f4")
	for i, spanID := range []string{
		"0000000000000001", "8000000000000000", "ffffffffffffffff", "",
	} {
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(i + 1))
		ex.SetDoubleValue(float64(i + 1))
		ex.SetTraceID(traceID)
		if spanID != "" {
			ex.SetSpanID(storetest.MustDecodeSpanID(spanID))
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	got := getMetricFullByName(t, s, ctx, "exemplar_span_ids")
	datapoints := metricDatapoints(got)
	require.Len(t, datapoints, 1)
	exemplars := datapoints[0].(map[string]any)["exemplars"].([]any)
	require.Len(t, exemplars, 4)
	byValue := make(map[float64]any, len(exemplars))
	for _, raw := range exemplars {
		ex := raw.(map[string]any)
		byValue[ex["doubleValue"].(float64)] = ex["spanID"]
	}
	require.Equal(t, "0000000000000001", byValue[1])
	require.Equal(t, "8000000000000000", byValue[2])
	require.Equal(t, "ffffffffffffffff", byValue[3])
	require.Nil(t, byValue[4])

	var nullSpanIDs int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select count(*) from exemplars where span_id is null`).Scan(&nullSpanIDs)
	}))
	require.Equal(t, 1, nullSpanIDs)
}

func TestExemplarValuesRoundTripByType(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "typed-exemplars")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("typed_exemplars")
	gauge := m.SetEmptyGauge()
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC).UnixNano()

	tests := []struct {
		name        string
		setValue    func(pmetric.Exemplar)
		valueType   string
		doubleValue any
		intValue    any
	}{
		{"double", func(ex pmetric.Exemplar) { ex.SetDoubleValue(1.25) }, "Double", 1.25, nil},
		{"negative zero", func(ex pmetric.Exemplar) { ex.SetDoubleValue(math.Copysign(0, -1)) }, "Double", "0x8000000000000000", nil},
		{"NaN", func(ex pmetric.Exemplar) { ex.SetDoubleValue(math.Float64frombits(0x7ff8000000000001)) }, "Double", "0x7ff8000000000001", nil},
		{"positive infinity", func(ex pmetric.Exemplar) { ex.SetDoubleValue(math.Inf(1)) }, "Double", "0x7ff0000000000000", nil},
		{"negative infinity", func(ex pmetric.Exemplar) { ex.SetDoubleValue(math.Inf(-1)) }, "Double", "0xfff0000000000000", nil},
		{"integer zero", func(ex pmetric.Exemplar) { ex.SetIntValue(0) }, "Int", nil, "0"},
		{"above 2^53", func(ex pmetric.Exemplar) { ex.SetIntValue(9_007_199_254_740_993) }, "Int", nil, "9007199254740993"},
		{"minimum int64", func(ex pmetric.Exemplar) { ex.SetIntValue(-1 << 63) }, "Int", nil, "-9223372036854775808"},
		{"maximum int64", func(ex pmetric.Exemplar) { ex.SetIntValue(1<<63 - 1) }, "Int", nil, "9223372036854775807"},
		{"empty", func(pmetric.Exemplar) {}, "Empty", nil, nil},
	}
	for i, tc := range tests {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base + int64(i)*int64(time.Second)))
		dp.SetDoubleValue(1)
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(base + int64(i)))
		tc.setValue(ex)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	got := getMetricFullByName(t, s, ctx, "typed_exemplars")
	byTimestamp := make(map[string]map[string]any, len(tests))
	for _, rawDP := range metricDatapoints(got) {
		exemplars := rawDP.(map[string]any)["exemplars"].([]any)
		require.Len(t, exemplars, 1)
		ex := exemplars[0].(map[string]any)
		byTimestamp[ex["timestamp"].(string)] = ex
	}
	require.Len(t, byTimestamp, len(tests))

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := byTimestamp[strconv.FormatInt(base+int64(i), 10)]
			require.NotNil(t, ex)
			assert.Equal(t, tc.valueType, ex["valueType"])
			assert.Equal(t, tc.doubleValue, ex["doubleValue"])
			assert.Equal(t, tc.intValue, ex["intValue"])
		})
	}

	received := getExactMetricSeriesByName(t, s, ctx, "typed_exemplars", store.TimeRange{})
	receivedByTimestamp := make(map[string]map[string]any, len(tests))
	for _, rawDP := range exactMetricSeriesDatapoints(t, received) {
		dp := rawDP.(map[string]any)
		require.NotContains(t, dp, "exemplarCount")
		exemplars := dp["exemplars"].([]any)
		require.Len(t, exemplars, 1)
		ex := exemplars[0].(map[string]any)
		receivedByTimestamp[ex["timestamp"].(string)] = ex
	}
	require.Equal(t, byTimestamp, receivedByTimestamp)
}

func requireMetric(t *testing.T, m map[string]any, name string) {
	t.Helper()
	assert.NotNil(t, m, "metric %q not found", name)
	if m != nil {
		assert.Equal(t, name, m["name"], "metric name")
	}
}

// metricDatapoints flattens m["timeseries"][*]["datapoints"][*] into a
// single []any in the order the SQL emits them: timeseries sorted by
// latest dp timestamp desc, datapoints within each timeseries sorted
// by timestamp desc. Tests that don't care about per-timeseries
// grouping use this to keep their assertions terse; tests that DO care
// about grouping should walk m["timeseries"] directly.
func metricDatapoints(m map[string]any) []any {
	if m == nil {
		return nil
	}
	timeseries, _ := m["timeseries"].([]any)
	out := make([]any, 0)
	for _, ts := range timeseries {
		ts, _ := ts.(map[string]any)
		if ts == nil {
			continue
		}
		dps, _ := ts["datapoints"].([]any)
		out = append(out, dps...)
	}
	return out
}

// deleteByIdentity is a thin test helper that resolves the exact OTel
// identity to a Metric reference and then calls
// DeleteMetric. The production JSON-RPC layer does the same
// resolve-then-delete pattern; we replicate it here so the existing
// test cases stay readable without needing to spell out metricIDs.
// deleteByIdentity resolves an identity tuple to a Metric reference and deletes
// it. Both steps run in one write-lock window so the resolved ID cannot be
// pruned out from under the delete.
func deleteByIdentity(t *testing.T, ctx context.Context, s *store.Store, name, unit, metricType, aggTemporality, isMonotonic, scopeName, scopeVersion, serviceName string) error {
	t.Helper()
	temporalityCode := map[string]int32{
		"": 0, "Unspecified": int32(pmetric.AggregationTemporalityUnspecified),
		"Delta":      int32(pmetric.AggregationTemporalityDelta),
		"Cumulative": int32(pmetric.AggregationTemporalityCumulative),
	}[aggTemporality]
	const q = `
		select m.id::varchar from metrics m
		where m.name = ?
		  and m.unit = ?
		  and m.metric_type = ?
		  and m.aggregation_temporality = ?
		  and m.is_monotonic = ?
		  and m.scope.name = ?
		  and m.scope.version = ?
		  and m.service_name = ?
		limit 1
	`
	return s.WithDBWrite(func(db *sql.DB) error {
		var metricID string
		err := db.QueryRowContext(ctx, q,
			name, unit, metricType, temporalityCode, isMonotonic == "true",
			scopeName, scopeVersion, serviceName,
		).Scan(&metricID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return metrics.DeleteMetric(ctx, db, metricID)
	})
}

// TestDeleteMetric covers the per-Metric cascade. Each subtest
// ingests a fixture, resolves an identity tuple to a Metric reference, calls
// DeleteMetric, and checks that (a) every row backing that Metric
// is gone and (b) nothing else was touched.
func TestDeleteMetric(t *testing.T) {
	t.Parallel()
	t.Run("removes a single Gauge by name+unit+scope+service", func(t *testing.T) {
		s, ctx := storetest.New(t)

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)

		before := searchMetricsAll(t, s, ctx)
		assert.Len(t, before, 5)

		// Gauges have no temporality / monotonic; pass empty strings.
		err = deleteByIdentity(t, ctx, s,
			"gauge_metric", "bytes", "Gauge",
			"", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		after := searchMetricsAll(t, s, ctx)
		assert.Len(t, after, 4)
		for _, m := range after {
			assert.NotEqual(t, "gauge_metric", m["name"])
		}

		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from metrics where name = ?`, "gauge_metric"))
	})

	t.Run("collapses multiple ingestions of the same logical metric", func(t *testing.T) {
		s, ctx := storetest.New(t)

		for i := 0; i < 3; i++ {
			err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, createTestMetricsPdata())
			})
			assert.NoError(t, err)
		}

		assert.Len(t, searchSummariesAll(t, s, ctx), 5)
		assert.Equal(t, 5, storetest.CountRows(t, s, ctx, `select count(*) from metrics`))
		assert.Equal(t, 15, storetest.CountRows(t, s, ctx, `select count(*) from metric_datapoints`))

		err := deleteByIdentity(t, ctx, s,
			"gauge_metric", "bytes", "Gauge",
			"", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Len(t, searchSummariesAll(t, s, ctx), 4)
		assert.Equal(t, 4, storetest.CountRows(t, s, ctx, `select count(*) from metrics`))
		assert.Equal(t, 12, storetest.CountRows(t, s, ctx, `select count(*) from metric_datapoints`))
		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from metrics where name = ?`, "gauge_metric"))
		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from metric_datapoints d join metrics s on s.id = d.metric_id where s.name = ?`,
			"gauge_metric"))
	})

	t.Run("unit discriminates same-name metrics", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		// Same name, different units.
		for _, unit := range []string{"bytes", "count"} {
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit(unit)
			g := m.SetEmptyGauge()
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 2)

		// Delete the bytes one — count should survive.
		err = deleteByIdentity(t, ctx, s,
			"requests", "bytes", "Gauge", "", "",
			"scope", "v1", "svc",
		)
		assert.NoError(t, err)

		after := searchMetricsAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, "count", after[0]["unit"])
	})

	t.Run("service.name discriminates same-name metrics from different services", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		for _, svc := range []string{"svc-a", "svc-b"} {
			rm := md.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", svc)
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName("scope")
			sm.Scope().SetVersion("v1")
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit("count")
			g := m.SetEmptyGauge()
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s,
			"requests", "count", "Gauge", "", "",
			"scope", "v1", "svc-a",
		)
		assert.NoError(t, err)

		// Summaries expose serviceName at the top level.
		after := searchSummariesAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, "svc-b", after[0]["serviceName"])
	})

	t.Run("is_monotonic discriminates Sum metrics", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		// Same name + unit + temporality, different monotonic flags.
		for _, monotonic := range []bool{true, false} {
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit("count")
			sum := m.SetEmptySum()
			sum.SetIsMonotonic(monotonic)
			sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			dp := sum.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s,
			"requests", "count", "Sum", "Cumulative", "true",
			"scope", "v1", "svc",
		)
		assert.NoError(t, err)

		// isMonotonic is reported by SearchSummaries (not by the Search
		// detail endpoint, which nests it inside per-datapoint payloads).
		after := searchSummariesAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, false, after[0]["isMonotonic"])
	})

	t.Run("no-match identity is a no-op", func(t *testing.T) {
		s, ctx := storetest.New(t)

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 5)

		err = deleteByIdentity(t, ctx, s,
			"nonexistent", "bytes", "Gauge", "", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 5)
	})

	t.Run("cascade removes attributes, exemplars, datapoints", func(t *testing.T) {
		s, ctx := storetest.New(t)

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)

		// Histogram has datapoints with exemplars — pick it.
		dpBefore := storetest.CountRows(t, s, ctx,
			`select count(*) from metric_datapoints where metric_id in (select id from metrics where name = ?)`,
			"histogram_metric")
		exBefore := storetest.CountRows(t, s, ctx,
			`select count(*) from exemplars where metric_datapoint_id in (select id from metric_datapoints where metric_id in (select id from metrics where name = ?))`,
			"histogram_metric")
		attrBefore := storetest.CountRows(t, s, ctx,
			`select count(distinct t.key)
			 from metrics m,
			 unnest(variant_keys(m.resource.attributes) || variant_keys(m.scope.attributes)) as t(key)
			 where m.name = ?`,
			"histogram_metric")
		assert.Greater(t, dpBefore, 0)
		assert.Greater(t, exBefore, 0)
		assert.Greater(t, attrBefore, 0)

		// Other Metrics must retain their independently owned Resource payloads.
		otherResources := storetest.CountRows(t, s, ctx, `select count(*) from metrics where name <> 'histogram_metric' and len(variant_keys(resource.attributes)) > 0`)
		assert.Greater(t, otherResources, 0)

		err = deleteByIdentity(t, ctx, s,
			"histogram_metric", "seconds", "Histogram",
			"Delta", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from metrics where name = ?`, "histogram_metric"))
		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from metric_datapoints where metric_id in (select id from metrics where name = ?)`,
			"histogram_metric"))
		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from exemplars e where exists (
				select 1 from metric_datapoints d
				where d.id = e.metric_datapoint_id
				  and d.metric_id in (select id from metrics where name = ?)
			)`, "histogram_metric"))
		assert.Equal(t, otherResources, storetest.CountRows(t, s, ctx, `select count(*) from metrics where len(variant_keys(resource.attributes)) > 0`),
			"a Metric delete must preserve other Metrics' owned payloads")

		// Sweep only collects histogram bounds abandoned by the delete.
		// Resource and Scope remain embedded on the four surviving Metrics.
		assert.Equal(t, 0, storetest.CountRows(t, s, ctx,
			`select count(*) from duckdb_tables() where table_name in ('attributes', 'resources', 'scopes')`),
			"owned payloads require no auxiliary attribute or context tables")
		assert.Greater(t, storetest.CountRows(t, s, ctx, `select count(distinct resource.id) from metrics`), 0,
			"the resource is still referenced by the four surviving Metrics")
	})
}

// TestMetrics_FindOrInsertIdempotent verifies that repeated exact Metrics
// resolve to stable Metric and series references.
func TestMetrics_FindOrInsertIdempotent(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	const batches = 5
	for i := 0; i < batches; i++ {
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 5, storetest.CountRows(t, s, ctx,
		`select count(*) from metrics`),
		"distinct logical metrics should not multiply across batches")
	assert.Equal(t, 5*batches, storetest.CountRows(t, s, ctx, `select count(*) from metric_datapoints`))

	gaugeMetricRows := storetest.CountRows(t, s, ctx,
		`select count(distinct d.metric_id) from metric_datapoints d
		 join metrics m on m.id = d.metric_id
		 where m.name = 'gauge_metric'`)
	assert.Equal(t, 1, gaugeMetricRows,
		"all gauge_metric ingests must share one metric_id")

	// Sanity: cross-table referential integrity holds.
	orphanDatapoints := storetest.CountRows(t, s, ctx,
		`select count(*) from metric_datapoints d
		 left join metrics s on s.id = d.metric_id
		 where s.id is null`)
	assert.Equal(t, 0, orphanDatapoints, "no datapoint may dangle after dedup")
}

// TestMetrics_DistinctIdentitiesStayDistinct verifies that changing any
// identifying field produces a distinct metric row.
func TestMetrics_DistinctIdentitiesStayDistinct(t *testing.T) {
	t.Parallel()
	mk := func(t *testing.T, mutate func(m pmetric.Metric, sm pmetric.ScopeMetrics, rm pmetric.ResourceMetrics)) pmetric.Metrics {
		t.Helper()
		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc-a")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		m := sm.Metrics().AppendEmpty()
		m.SetName("requests")
		m.SetUnit("count")
		m.SetEmptySum().SetIsMonotonic(true)
		m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		dp := m.Sum().DataPoints().AppendEmpty()
		dp.SetIntValue(1)
		mutate(m, sm, rm)
		return md
	}

	cases := []struct {
		name   string
		mutate func(m pmetric.Metric, sm pmetric.ScopeMetrics, rm pmetric.ResourceMetrics)
	}{
		{"name", func(m pmetric.Metric, _ pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) { m.SetName("requests_v2") }},
		{"unit", func(m pmetric.Metric, _ pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) { m.SetUnit("ms") }},
		{"type", func(m pmetric.Metric, _ pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) { m.SetEmptyGauge() }},
		{"temporality", func(m pmetric.Metric, _ pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		}},
		{"is_monotonic", func(m pmetric.Metric, _ pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			m.Sum().SetIsMonotonic(false)
		}},
		{"scope_name", func(_ pmetric.Metric, sm pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			sm.Scope().SetName("scope-b")
		}},
		{"scope_version", func(_ pmetric.Metric, sm pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			sm.Scope().SetVersion("v2")
		}},
		{"scope_schema_url", func(_ pmetric.Metric, sm pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			sm.SetSchemaUrl("schema-b")
		}},
		{"scope_attribute", func(_ pmetric.Metric, sm pmetric.ScopeMetrics, _ pmetric.ResourceMetrics) {
			sm.Scope().Attributes().PutStr("build", "blue")
		}},
		{"resource_attribute", func(_ pmetric.Metric, _ pmetric.ScopeMetrics, rm pmetric.ResourceMetrics) {
			rm.Resource().Attributes().PutStr("host.name", "pod-b")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := storetest.New(t)

			err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, mk(t, func(pmetric.Metric, pmetric.ScopeMetrics, pmetric.ResourceMetrics) {}))
			})
			require.NoError(t, err)

			err = s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, mk(t, tc.mutate))
			})
			require.NoError(t, err)

			assert.Equal(t, 2, storetest.CountRows(t, s, ctx,
				`select count(*) from metrics`),
				"changing %s should produce a distinct Metric", tc.name)
		})
	}
}

func TestMetrics_NonIdentifyingVariantsReuseIdentity(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	makeBatch := func(variant bool) pmetric.Metrics {
		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("requests")
		metric.SetDescription("first")
		metric.Metadata().PutStr("owner", "first")
		metric.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
		if variant {
			metric.SetDescription("second")
			metric.Metadata().PutStr("owner", "second")
		}
		return md
	}
	for _, variant := range []bool{false, true} {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, makeBatch(variant))
		}))
	}
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(*) from metrics`))
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(distinct resource.id) from metrics`))
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(distinct scope.id) from metrics`))
	assert.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(*) from metric_series`))
	var metricID string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select id::varchar from metrics`).Scan(&metricID)
	}))
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricOTLP(ctx, db, metricID)
	})
	require.NoError(t, err)
	text := string(raw)
	assert.Contains(t, text, `"description":"second"`)
	assert.Contains(t, text, `"description":"first"`, "exports retain earlier received descriptors")
}

func TestMetricRecordIdentityIsUnique(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		_, err := db.Exec(`insert into metric_records select * from metric_records where name = 'gauge_metric'`)
		require.ErrorContains(t, err, "Duplicate key")
		_, err = db.Exec(`insert into metric_records select * replace(uuid() as record_id) from metric_records where name = 'gauge_metric'`)
		require.NoError(t, err, "repeated logical identities retain separate received records")
		return nil
	}))
}

// TestMetrics_ServiceNameDenormStaysConsistent verifies the
// invariant that justifies denormalizing service.name as a column
// alongside its source-of-truth attribute row: for every metrics
// row, the column value must equal the resource attribute value that
// produced it. If we ever break this (e.g. by writing only the column
// and dropping the attribute, or by ingesting two batches with
// inconsistent service names for the same identity), this test will
// catch it.
func TestMetrics_ServiceNameDenormStaysConsistent(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	require.NoError(t, err)

	mismatches := storetest.CountRows(t, s, ctx, `
		select count(*) from metrics s
		where s.service_name <> coalesce(s.resource.attributes['service.name']::varchar, '')
	`)
	assert.Equal(t, 0, mismatches,
		"metrics.service_name must equal the source resource attribute")
}

// TestEmptyMetrics verifies empty metric list and empty store.
func TestEmptyMetrics(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, pmetric.NewMetrics())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
}

// TestClearMetrics verifies that all metrics can be cleared, including child rows.
func TestClearMetrics(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 5)
	assert.Greater(t, storetest.CountRows(t, s, ctx, "select count(*) from metric_datapoints"), 0)
	assert.Greater(t, storetest.CountRows(t, s, ctx, "select count(*) from metric_datapoints, unnest(variant_keys(attributes))"), 0)

	err = s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	})
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
	assert.Equal(t, 0, storetest.CountRows(t, s, ctx, "select count(*) from metrics"))
	assert.Equal(t, 0, storetest.CountRows(t, s, ctx, "select count(*) from metric_series"))
	assert.Equal(t, 0, storetest.CountRows(t, s, ctx, "select count(*) from metric_datapoints"))
	assert.Equal(t, 0, storetest.CountRows(t, s, ctx, "select count(*) from exemplars"))

	assert.Zero(t, storetest.CountRows(t, s, ctx, "select count(*) from metric_records"), "owned bounds disappear with records")
}

func TestExpHistogramZeroThresholdRoundTrip(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	ts := time.Unix(1700000000, 0)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "zt-test")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")

	// Metric 1: defaults (zero_threshold should land as 0).
	m1 := sm.Metrics().AppendEmpty()
	m1.SetName("exphist_zt_default")
	exp1 := m1.SetEmptyExponentialHistogram()
	exp1.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp1 := exp1.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
	dp1.SetCount(10)
	dp1.SetSum(5.0)
	dp1.SetScale(2)
	dp1.SetZeroCount(1)
	dp1.Positive().SetOffset(0)
	dp1.Positive().BucketCounts().FromRaw([]uint64{4, 5})
	dp1.Negative().SetOffset(0)
	dp1.Negative().BucketCounts().FromRaw([]uint64{})

	// Metric 2: explicit non-zero threshold; the merge story in 3c needs
	// this value preserved exactly.
	m2 := sm.Metrics().AppendEmpty()
	m2.SetName("exphist_zt_set")
	exp2 := m2.SetEmptyExponentialHistogram()
	exp2.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp2 := exp2.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
	dp2.SetCount(10)
	dp2.SetSum(5.0)
	dp2.SetScale(2)
	dp2.SetZeroCount(1)
	dp2.SetZeroThreshold(0.001)
	dp2.Positive().SetOffset(0)
	dp2.Positive().BucketCounts().FromRaw([]uint64{4, 5})
	dp2.Negative().SetOffset(0)
	dp2.Negative().BucketCounts().FromRaw([]uint64{})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	byName := make(map[string]map[string]any)
	for _, summary := range searchMetricsAll(t, s, ctx) {
		name, _ := summary["name"].(string)
		byName[name] = getMetricFullByName(t, s, ctx, name)
	}

	// Helper: pull the first datapoint from a metric and return its
	// zeroThreshold value.
	zeroThreshold := func(metricName string) any {
		m, ok := byName[metricName]
		require.True(t, ok, "metric %s not in search results", metricName)
		dps := metricDatapoints(m)
		require.Len(t, dps, 1)
		dp, _ := dps[0].(map[string]any)
		require.Contains(t, dp, "zeroThreshold", "zeroThreshold missing from output JSON")
		return dp["zeroThreshold"]
	}

	// Default case: pdata's default is 0, so ingest writes 0.
	got := zeroThreshold("exphist_zt_default")
	gotF, ok := got.(float64)
	require.True(t, ok, "expected number, got %T", got)
	assert.InDelta(t, 0.0, gotF, 1e-12)

	// Explicit case: 0.001 should round-trip exactly.
	got = zeroThreshold("exphist_zt_set")
	gotF, ok = got.(float64)
	require.True(t, ok, "expected number, got %T", got)
	assert.InDelta(t, 0.001, gotF, 1e-12)
}

// makeMergedHistogramFixture builds a pmetric.Metrics with one
// Histogram metric (Delta temporality) containing the given datapoints. Used
// by the merged quantile series tests so each subtest can compose its
// own scenario (multi-Metric, multi-timestamp, bounds mismatch) without
// perturbing the shared createTestMetricsPdata fixture.
func makeMergedHistogramFixture(name string, dps []histTestDP) pmetric.Metrics {
	return makeHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, dps)
}

// makeHistogramFixtureT is the temporality-parameterized variant of
// makeMergedHistogramFixture. Bucketing tests use this to exercise both
// Delta (within-bucket sum) and Cumulative (within-bucket arg_max-latest)
// dispatch paths.
type sumTestDP struct {
	timestamp time.Time
	value     float64
	attrs     map[string]string
}

// makeSumFixtureT builds a single-series Sum, which is what the scalar view
// grid aggregates over.
func makeSumFixtureT(name string, temporality pmetric.AggregationTemporality, dps []sumTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-sum")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(temporality)
	for _, d := range dps {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(d.timestamp.UnixNano()))
		dp.SetDoubleValue(d.value)
		for k, v := range d.attrs {
			dp.Attributes().PutStr(k, v)
		}
	}
	return md
}

func makeHistogramFixtureT(name string, temporality pmetric.AggregationTemporality, dps []histTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-merged")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	hist := m.SetEmptyHistogram()
	hist.SetAggregationTemporality(temporality)
	for _, dp := range dps {
		h := hist.DataPoints().AppendEmpty()
		h.SetTimestamp(pcommon.Timestamp(dp.timestamp.UnixNano()))
		h.SetCount(dp.count)
		h.SetSum(dp.sum)
		h.SetMin(dp.min)
		h.SetMax(dp.max)
		h.BucketCounts().FromRaw(dp.counts)
		h.ExplicitBounds().FromRaw(dp.bounds)
		for k, v := range dp.attrs {
			h.Attributes().PutStr(k, v)
		}
	}
	return md
}

// expHistTestDP is the ExpHistogram analogue of histTestDP. Tests build
// these for the bucketing + alignment paths.
type expHistTestDP struct {
	timestamp     time.Time
	attrs         map[string]string
	scale         int32
	zeroCount     uint64
	zeroThreshold float64
	posOffset     int32
	posCounts     []uint64
	negOffset     int32
	negCounts     []uint64
	count         uint64
	sum           float64
	min           float64
	max           float64
}

// makeExpHistogramFixtureT builds a single-metric ExpHistogram fixture with
// the given datapoints and temporality.
func makeExpHistogramFixtureT(name string, temporality pmetric.AggregationTemporality, dps []expHistTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-exphist")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	exp := m.SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(temporality)
	for _, dp := range dps {
		e := exp.DataPoints().AppendEmpty()
		e.SetTimestamp(pcommon.Timestamp(dp.timestamp.UnixNano()))
		e.SetCount(dp.count)
		e.SetSum(dp.sum)
		e.SetMin(dp.min)
		e.SetMax(dp.max)
		e.SetScale(dp.scale)
		e.SetZeroCount(dp.zeroCount)
		e.SetZeroThreshold(dp.zeroThreshold)
		e.Positive().SetOffset(dp.posOffset)
		e.Positive().BucketCounts().FromRaw(dp.posCounts)
		e.Negative().SetOffset(dp.negOffset)
		e.Negative().BucketCounts().FromRaw(dp.negCounts)
		for k, v := range dp.attrs {
			e.Attributes().PutStr(k, v)
		}
	}
	return md
}

// histTestDP is a compact builder for one Histogram datapoint, used only
// in tests. Mirrors the shape of pmetric.HistogramDataPoint with the
// fields we actually exercise.
type histTestDP struct {
	timestamp time.Time
	attrs     map[string]string
	bounds    []float64
	counts    []uint64
	count     uint64
	sum       float64
	min       float64
	max       float64
}

// findMetricRef looks up the ingested metric's UUID by name via Search. The
// id is generated at ingest time so we can't predict it.
func findMetricRef(t *testing.T, s *store.Store, ctx context.Context, name string) string {
	t.Helper()
	for _, m := range searchMetricsAll(t, s, ctx) {
		if m["name"] == name {
			id, _ := m["metricRef"].(string)
			return id
		}
	}
	t.Fatalf("metric %q not found", name)
	return ""
}

// TestIngestMetrics_LargeBatchStaysConsistent verifies independently owned
// records and their attributes within one received batch.
func TestIngestMetrics_LargeBatchStaysConsistent(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	const batchSize = 101
	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdataN(batchSize))
	})
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, s, ctx)
	assert.Len(t, metrics, batchSize)

	for _, idx := range []int{0, 99, 100} {
		name := "flush_metric_" + fmt.Sprintf("%d", idx)
		m := getMetricFullByName(t, s, ctx, name)
		resource, _ := m["resource"].(map[string]any)
		assert.NotNil(t, resource)
		attrs, _ := resource["attributes"].([]any)
		var resourceKey string
		for _, a := range attrs {
			kv, _ := a.(map[string]any)
			if k, _ := kv["key"].(string); k == "resource.key" {
				resourceKey, _ = kv["value"].(map[string]any)["value"].(string)
				break
			}
		}
		assert.Equal(t, "resource.val", resourceKey, "metric %s resource.key", name)
		scope, _ := m["scope"].(map[string]any)
		assert.NotNil(t, scope)
		scopeAttrs, _ := scope["attributes"].([]any)
		var scopeKey string
		for _, a := range scopeAttrs {
			kv, _ := a.(map[string]any)
			if k, _ := kv["key"].(string); k == "scope.key" {
				scopeKey, _ = kv["value"].(map[string]any)["value"].(string)
				break
			}
		}
		assert.Equal(t, "scope.val", scopeKey, "metric %s scope.key", name)
	}
}

func TestIngest_CanceledContext(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var rejected ingest.Rejected
	err := s.WithConn(func(conn driver.Conn) error {
		var err error
		rejected, err = metrics.IngestReport(ctx, conn, createTestMetricsPdataN(1))
		return err
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, rejected.Count())
}

func TestIngest_CanceledDuringIngest(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	ctx, cancel := context.WithCancel(context.Background())

	var rejected ingest.Rejected
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.WithConn(func(conn driver.Conn) error {
			var err error
			rejected, err = metrics.IngestReport(ctx, conn, createTestMetricsPdataN(100))
			return err
		})
	}()
	cancel()

	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, rejected.Count())
	for _, table := range []string{"metrics", "metric_series", "metric_datapoints"} {
		assert.Equal(t, 0, storetest.CountRows(t, s, context.Background(), `select count(*) from `+table),
			"canceled ingest must not leave provisional rows in %s", table)
	}
}

// Complete Resource identity keeps replicas of one service in distinct Metrics
// and chart series, even when their instrument and datapoint labels match.
func buildTwoReplicaMetrics(t *testing.T) pmetric.Metrics {
	t.Helper()
	md := pmetric.NewMetrics()
	base := time.Now().UnixNano()

	// Same service, same scope, same metric, same datapoint labels. The
	// resources differ by service.instance.id -- the identifying field -- and,
	// incidentally, by host.name too.
	for i, host := range []string{"pod-a", "pod-b"} {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		rm.Resource().Attributes().PutStr("service.instance.id", "checkout-"+host)
		rm.Resource().Attributes().PutStr("host.name", host)

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("otelhttp")
		sm.Scope().SetVersion("1.2.0")

		m := sm.Metrics().AppendEmpty()
		m.SetName("http.server.duration")
		m.SetUnit("ms")
		g := m.SetEmptyGauge()
		for j := 0; j < 3; j++ {
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
			dp.SetDoubleValue(float64(10*(i+1) + j))
			dp.Attributes().PutStr("http.route", "/checkout")
		}
	}
	return md
}

// TestMetricSeries_SplitByResource verifies that distinct service instances
// produce distinct resource rows and series.
func TestMetricSeries_SplitByResource(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t))
	}))

	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, "select count(*) from (select distinct resource.id from metrics)"),
		"distinct resource payloads must produce distinct resource rows")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2, "each Resource identifies a separate OTel Metric")
	for _, summary := range summaries {
		assert.Equal(t, float64(1), summary["seriesCount"])
		metricID := summary["metricRef"].(string)
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)), 0, nil, nil, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		ts := metric["timeseries"].([]any)
		require.Len(t, ts, 1)
		assert.Len(t, ts[0].(map[string]any)["datapoints"].([]any), 3)
	}
}

// Resource attributes are part of OTLP metric identity. Two payloads that
// differ only by host.name remain two series even when neither has a
// service.instance.id; storage must not invent a coarser resource identity.
func TestMetricSeries_ResourceOnlyDiffersByHostNameSplits(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	md := pmetric.NewMetrics()
	base := time.Now().UnixNano()
	for i, host := range []string{"pod-a", "pod-b"} {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		rm.Resource().Attributes().PutStr("host.name", host)

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("otelhttp")
		sm.Scope().SetVersion("1.2.0")

		m := sm.Metrics().AppendEmpty()
		m.SetName("http.server.duration")
		m.SetUnit("ms")
		g := m.SetEmptyGauge()
		for j := 0; j < 3; j++ {
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
			dp.SetDoubleValue(float64(10*(i+1) + j))
			dp.Attributes().PutStr("http.route", "/checkout")
		}
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, "select count(*) from (select distinct resource.id from metrics)"),
		"distinct received resource payloads must remain distinct")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2)
	for _, summary := range summaries {
		assert.Equal(t, float64(1), summary["seriesCount"])
	}
}

// A series id has to survive re-ingest and retention within one persisted
// database, because it is what a shared URL names.
func TestMetricSeries_IDsAreStableAcrossReingest(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	seriesIDs := func() []string {
		var out []string
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.Query(`select id::varchar from metric_series order by 1`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				out = append(out, id)
			}
			return rows.Err()
		}))
		return out
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t))
	}))
	first := seriesIDs()
	require.Len(t, first, 2, "two replicas are two series")

	// The same content again: same ids, no new rows.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t))
	}))
	assert.Equal(t, first, seriesIDs(),
		"re-ingesting the same series must not mint new ids")

	// And the id the wire serves is the id in the table, or a URL built from
	// one could not be resolved back to the other.
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2)
	var served []string
	for _, summary := range summaries {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summary["metricRef"].(string), store.BoundedTimeRange(0,
				time.Now().UnixNano()+int64(time.Hour)), 0, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		served = append(served, metric["timeseries"].([]any)[0].(map[string]any)["seriesRef"].(string))
	}
	sort.Strings(served)
	assert.Equal(t, first, served,
		"the key on the wire must be the series id, so a link resolves back to a row")

	other, otherCtx := storetest.New(t)
	require.NoError(t, other.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(otherCtx, conn, buildTwoReplicaMetrics(t))
	}))
	metricIDs := func(rows []map[string]any) []string {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row["metricRef"].(string))
		}
		sort.Strings(ids)
		return ids
	}
	assert.Equal(t, metricIDs(summaries), metricIDs(searchMetricsAll(t, other, otherCtx)),
		"logical Metric identities are deterministic across stores")
	var fresh []string
	require.NoError(t, other.WithDBRead(func(db *sql.DB) error {
		rows, err := db.Query(`select id::varchar from metric_series order by 1`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			fresh = append(fresh, id)
		}
		return rows.Err()
	}))
	assert.Equal(t, first, fresh, "series identities are deterministic across stores")
}

func TestExactMetricAndSeriesIDsSurvivePersistentReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metric-identity.db")
	ctx := context.Background()
	open := func() *store.Store {
		s, err := store.NewStore(ctx, path, zap.NewNop())
		require.NoError(t, err)
		return s
	}
	readIDs := func(s *store.Store, table string) []string {
		var ids []string
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.Query(`select id::varchar from ` + table + ` order by 1`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		}))
		return ids
	}

	first := open()
	require.NoError(t, first.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t))
	}))
	metricIDs := readIDs(first, "metrics")
	seriesIDs := readIDs(first, "metric_series")
	require.Len(t, metricIDs, 2)
	require.Len(t, seriesIDs, 2)
	require.NoError(t, first.Close())

	reopened := open()
	assert.Equal(t, metricIDs, readIDs(reopened, "metrics"))
	assert.Equal(t, seriesIDs, readIDs(reopened, "metric_series"))
	require.NoError(t, reopened.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t))
	}))
	assert.Equal(t, metricIDs, readIDs(reopened, "metrics"))
	assert.Equal(t, seriesIDs, readIDs(reopened, "metric_series"))
	assert.Equal(t, 12, storetest.CountRows(t, reopened, ctx, `select count(*) from metric_datapoints`))
	require.NoError(t, reopened.Close())
}

// buildInstanceMetrics emits one gauge from one instance, optionally with extra
// resource attributes bolted on -- the shape a collector processor produces
// when it starts resolving metadata partway through a Metric's lifetime.
func buildInstanceMetrics(t *testing.T, extra map[string]string, dropped uint32, base int64) pmetric.Metrics {
	t.Helper()
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	rm.Resource().Attributes().PutStr("service.instance.id", "checkout-7f9c")
	rm.Resource().SetDroppedAttributesCount(dropped)
	for k, v := range extra {
		rm.Resource().Attributes().PutStr(k, v)
	}

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("otelhttp")
	sm.Scope().SetVersion("1.2.0")

	m := sm.Metrics().AppendEmpty()
	m.SetName("http.server.duration")
	m.SetUnit("ms")
	g := m.SetEmptyGauge()
	for j := 0; j < 3; j++ {
		dp := g.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
		dp.SetDoubleValue(float64(j))
		dp.Attributes().PutStr("http.route", "/checkout")
	}
	return md
}

// Adding Resource attributes changes the OTel Metric identity, so navigation
// and series selection must both keep the payloads separate.
func TestMetricSeries_SplitsWhenResourcePayloadChanges(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().UnixNano()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildInstanceMetrics(t, nil, 0, base))
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildInstanceMetrics(t, map[string]string{
			"telemetry.sdk.name":     "opentelemetry",
			"telemetry.sdk.language": "go",
			"telemetry.sdk.version":  "1.28.0",
		}, 0, base+10_000_000))
	}))

	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, "select count(*) from (select distinct resource.id from metrics)"),
		"distinct received resource payloads must remain distinguishable")
	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, "select count(*) from metric_series"),
		"resource attributes participate in OTLP metric series identity")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2)
	for _, summary := range summaries {
		assert.Equal(t, float64(1), summary["seriesCount"])
	}
}

func TestMetricSeries_DroppedResourceCountPreservesExactOwners(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Now().UnixNano()

	for i, dropped := range []uint32{0, 3} {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				buildInstanceMetrics(t, nil, dropped, base+int64(i)*10_000_000))
		}))
	}

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var resources, metricsCount, series int
		var minDropped, maxDropped uint32
		require.NoError(t, db.QueryRow(`select count(distinct resource.id) from metrics`).Scan(&resources))
		require.NoError(t, db.QueryRow(`select count(*) from metrics`).Scan(&metricsCount))
		require.NoError(t, db.QueryRow(`select count(*) from metric_series`).Scan(&series))
		require.NoError(t, db.QueryRow(`
			select min(resource.dropped_attributes_count), max(resource.dropped_attributes_count)
			from metrics`).Scan(&minDropped, &maxDropped))
		assert.Equal(t, 2, resources, "dropped-count payloads need distinct resource rows")
		assert.Equal(t, 2, metricsCount)
		assert.Equal(t, 2, series)
		assert.Equal(t, uint32(0), minDropped)
		assert.Equal(t, uint32(3), maxDropped)
		return nil
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2)
	seenDropped := map[float64]bool{}
	for _, summary := range summaries {
		assert.Equal(t, float64(1), summary["seriesCount"])
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summary["metricRef"].(string),
				store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)),
				0, nil, nil, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		dropped := metric["resourceDroppedAttributesCount"].(float64)
		seenDropped[dropped] = true
		resource := metric["resource"].(map[string]any)
		assert.Equal(t, dropped, resource["droppedAttributesCount"])
		timeseries := metric["timeseries"].([]any)
		require.Len(t, timeseries, 1)
		series := timeseries[0].(map[string]any)
		assert.Len(t, series["datapoints"].([]any), 3)
		assert.Equal(t, dropped, series["resource"].(map[string]any)["droppedAttributesCount"])
	}
	assert.Equal(t, map[float64]bool{0: true, 3: true}, seenDropped)
}

func ptrF(v float64) *float64 { return &v }

// mapKeys is the sorted key set of a decoded JSON object.
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestIngest_SingleBucketHistogram verifies the valid OTLP shape with one
// bucket and no explicit bounds. pdata represents its bounds as a nil slice.
func TestIngest_SingleBucketHistogram(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeHistogramFixtureT("single.bucket", pmetric.AggregationTemporalityDelta, []histTestDP{{
		timestamp: base,
		bounds:    nil,
		counts:    []uint64{7},
		count:     7, sum: 3.5, min: 0.5, max: 0.5,
	}})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}), "ingesting a single-bucket histogram must not fail")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1, "the metric must be readable after ingest")

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "the datapoint must survive the round trip")

	// The empty bounds vector must read back as an empty list, not null: the
	// renderer distinguishes "no bounds" from "bounds missing".
	dp, _ := dps[0].(map[string]any)
	require.Contains(t, dp, "explicitBounds")
	bounds, ok := dp["explicitBounds"].([]any)
	require.True(t, ok, "explicitBounds should be a list, got %T", dp["explicitBounds"])
	assert.Empty(t, bounds, "a single-bucket histogram has zero bounds")
}

// TestIngest_NilArraysReachingTheAppender verifies that nil histogram bucket
// slices are stored as empty DuckDB lists.
func TestIngest_NilArraysReachingTheAppender(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)

	t.Run("histogram with no buckets", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "nil-arrays")
		m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("no.buckets")
		h := m.SetEmptyHistogram()
		h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dp := h.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
		dp.SetCount(0)
		require.Nil(t, dp.BucketCounts().AsRaw(), "precondition: bucket_counts is nil, not empty")

		require.NoError(t, s.WithConn(func(c driver.Conn) error {
			return metrics.Ingest(ctx, c, md)
		}))
	})

	t.Run("exponential histogram with no negative buckets", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "nil-arrays")
		m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("no.negatives")
		e := m.SetEmptyExponentialHistogram()
		e.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dp := e.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
		dp.SetCount(3)
		dp.SetScale(0)
		dp.Positive().BucketCounts().FromRaw([]uint64{1, 2})
		require.Nil(t, dp.Negative().BucketCounts().AsRaw(), "precondition: negative buckets are nil")

		require.NoError(t, s.WithConn(func(c driver.Conn) error {
			return metrics.Ingest(ctx, c, md)
		}))
	})
}

// TestMetricMetadataRoundTrip verifies Metric metadata storage, display, and
// attribute discovery.
func TestMetricMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "metadata-test")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("requests.total")
	m.SetDescription("inbound requests")
	m.Metadata().PutStr("owner", "checkout-team")
	m.Metadata().PutStr("slo", "99.9")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	dp.SetDoubleValue(42)
	dp.Attributes().PutStr("route", "/cart")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	got := getMetricFullByName(t, s, ctx, "requests.total")

	meta, ok := got["metadata"].([]any)
	require.True(t, ok, "metadata must be present on the wire, got %T", got["metadata"])
	pairs := map[string]map[string]any{}
	for _, e := range meta {
		a := e.(map[string]any)
		pairs[a["key"].(string)] = a["value"].(map[string]any)
	}
	require.Equal(t, map[string]map[string]any{
		"owner": {"kind": "string", "value": "checkout-team"},
		"slo":   {"kind": "string", "value": "99.9"},
	}, pairs)

	// Datapoint labels and Metric metadata have separate attribute scopes.
	require.NotContains(t, pairs, "route")

	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`
			select count(*) from metrics m, unnest(variant_keys(m.metadata)) t(key)`).Scan(&n)
	}))
	require.Equal(t, 2, n, "both metadata attributes belong to the metadata scope")

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributeDefinitions(ctx, db)
	})
	require.NoError(t, err)
	var defs []map[string]any
	require.NoError(t, json.Unmarshal(raw, &defs))
	offered := false
	for _, d := range defs {
		if d["attributeScope"] == "metadata" && d["name"] == "owner" {
			offered = true
		}
	}
	require.True(t, offered, "metadata keys must be offered as searchable: %v", defs)
}

func TestMetricTimestampsRoundTripAcrossUint64Range(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	values := []uint64{0, 1<<63 - 1, 1 << 63, ^uint64(0)}

	data := pmetric.NewMetrics()
	metric := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("timestamp.boundaries")
	gauge := metric.SetEmptyGauge()
	for i, timestamp := range values {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetStartTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(int64(i))
		exemplar := dp.Exemplars().AppendEmpty()
		exemplar.SetTimestamp(pcommon.Timestamp(timestamp))
		exemplar.SetIntValue(int64(i))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select d.timestamp, d.start_time, e.timestamp
			from metric_datapoints d join exemplars e on e.metric_datapoint_id = d.id order by d.timestamp`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, want := range values {
			require.True(t, rows.Next())
			var timestamp, startTime, exemplar uint64
			require.NoError(t, rows.Scan(&timestamp, &startTime, &exemplar))
			require.Equal(t, want, timestamp)
			require.Equal(t, want, startTime)
			require.Equal(t, want, exemplar)
		}
		return rows.Err()
	}))

	summariesRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summariesRaw, &summaries))
	require.Equal(t, "18446744073709551615", summaries[0]["lastSeen"])

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"timestamp":"18446744073709551615"`)
	require.Contains(t, string(raw), `"startTime":"18446744073709551615"`)
}

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
		metric.SetName("same.metric")
		metric.SetUnit("1")
		sum := metric.SetEmptySum()
		sum.SetAggregationTemporality(temporality)
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(i + 1))
		dp.SetIntValue(int64(i))
	}
	gaugeMetric := sm.AppendEmpty()
	gaugeMetric.SetName("same.metric")
	gaugeMetric.SetUnit("1")
	gaugeDP := gaugeMetric.SetEmptyGauge().DataPoints().AppendEmpty()
	gaugeDP.SetTimestamp(9)
	gaugeDP.SetIntValue(9)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	var count, distinctCodes int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `select count(*), count(distinct aggregation_temporality)
			from metrics where name = 'same.metric'`).Scan(&count, &distinctCodes)
	}))
	assert.Equal(t, len(temporalities)+1, count)
	assert.Equal(t, len(temporalities), distinctCodes)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, 10), nil)
	})
	require.NoError(t, err)
	var summaries []struct {
		MetricRef                  string          `json:"metricRef"`
		MetricType                 string          `json:"metricType"`
		AggregationTemporalityCode json.RawMessage `json:"aggregationTemporalityCode"`
		AggregationTemporality     string          `json:"aggregationTemporality"`
	}
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Len(t, summaries, len(temporalities)+1, "metric type and unknown codes must remain distinct Metric identities")
	for _, summary := range summaries {
		if summary.MetricType == "Gauge" {
			assert.JSONEq(t, "null", string(summary.AggregationTemporalityCode))
			assert.Empty(t, summary.AggregationTemporality, "Gauge zero is non-applicable, not Unspecified")
		} else {
			var code int32
			require.NoError(t, json.Unmarshal(summary.AggregationTemporalityCode, &code))
			assert.Equal(t, labels[code], summary.AggregationTemporality)
		}
		detailRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summary.MetricRef, store.BoundedTimeRange(0, 10), 100, nil, nil, 0, 100, 100, nil, "UTC", nil, 100)
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

func TestSummaryRejectionPreservesAcceptedEmptyMetrics(t *testing.T) {
	for _, summaryFirst := range []bool{true, false} {
		name := "summary last"
		if summaryFirst {
			name = "summary first"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, ctx := storetest.New(t)
			batch := pmetric.NewMetrics()
			appendSummary := func() {
				rm := batch.ResourceMetrics().AppendEmpty()
				rm.Resource().Attributes().PutStr("rejected.resource", "summary")
				sm := rm.ScopeMetrics().AppendEmpty()
				sm.Scope().Attributes().PutBool("rejected.scope", true)
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("unsupported-summary")
				metric.Metadata().PutBool("rejected.metadata", true)
				metric.SetEmptySummary().DataPoints().AppendEmpty().SetCount(1)
			}
			if summaryFirst {
				appendSummary()
			}
			rm := batch.ResourceMetrics().AppendEmpty()
			rm.SetSchemaUrl("resource-schema")
			rm.Resource().Attributes().PutStr("service.name", "accepted")
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.SetSchemaUrl("scope-schema")
			sm.Scope().SetName("sdk")
			for _, metricType := range []pmetric.MetricType{
				pmetric.MetricTypeGauge, pmetric.MetricTypeSum,
				pmetric.MetricTypeHistogram, pmetric.MetricTypeExponentialHistogram,
			} {
				metric := sm.Metrics().AppendEmpty()
				metric.SetName(metricType.String())
				switch metricType {
				case pmetric.MetricTypeGauge:
					metric.SetEmptyGauge()
				case pmetric.MetricTypeSum:
					metric.SetEmptySum()
				case pmetric.MetricTypeHistogram:
					metric.SetEmptyHistogram()
				case pmetric.MetricTypeExponentialHistogram:
					metric.SetEmptyExponentialHistogram()
				}
			}
			repeated := sm.Metrics().AppendEmpty()
			repeated.SetName("Gauge")
			repeated.SetDescription("latest")
			repeated.SetEmptyGauge()
			if !summaryFirst {
				appendSummary()
			}

			var rejected ingest.Rejected
			require.NoError(t, s.WithConn(func(conn driver.Conn) error {
				var err error
				rejected, err = metrics.IngestReport(ctx, conn, batch)
				return err
			}))
			require.Equal(t, 1, rejected.Count())
			require.ErrorIs(t, rejected.Reason(), metrics.ErrUnsupportedMetricType)
			ids := metricMetricIDs(t, s, ctx)
			require.Len(t, ids, 4, "every accepted empty Metric must survive cleanup")
			require.NotContains(t, ids, "unsupported-summary")
			for name, id := range ids {
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetric(ctx, db, id)
				})
				require.NoError(t, err)
				var exact map[string]any
				require.NoError(t, json.Unmarshal(raw, &exact))
				assert.Equal(t, name, exact["name"])
				assert.Empty(t, exact["series"])
				if name == "Gauge" {
					assert.Equal(t, "latest", exact["description"])
				}
				otlp := getMetricOTLP(t, s, ctx, id)
				decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(otlp)
				require.NoError(t, err)
				wantRecords := 1
				if name == "Gauge" {
					wantRecords = 2
				}
				require.Equal(t, wantRecords, decoded.MetricCount(), "both received Gauge descriptors survive")
				assert.Zero(t, decoded.DataPointCount())
			}
			require.Equal(t, 0, storetest.CountRows(t, s, ctx, `select count(*) from metric_datapoints`))
			require.Equal(t, 0, storetest.CountRows(t, s, ctx, `select count(*) from metric_series`))
			require.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(distinct resource.id) from metrics`))
			require.Equal(t, 1, storetest.CountRows(t, s, ctx, `select count(distinct scope.id) from metrics`))
			require.Zero(t, storetest.CountRows(t, s, ctx, `select count(*) from metrics m,
				unnest(variant_keys(m.resource.attributes) || variant_keys(m.scope.attributes) || variant_keys(m.metadata)) t(key)
				where key like 'rejected.%'`))
		})
	}
}
