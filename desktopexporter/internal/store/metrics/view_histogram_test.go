package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc"
)

type optionalHistogramStatistic struct {
	present bool
	value   float64
}

type optionalHistogramDatapoint interface {
	SetTimestamp(pcommon.Timestamp)
	SetCount(uint64)
	SetSum(float64)
	SetMin(float64)
	SetMax(float64)
}

func TestHistogramOptionalStatisticsPreservePresence(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		timestamp uint64
		sum       optionalHistogramStatistic
		min       optionalHistogramStatistic
		max       optionalHistogramStatistic
	}{
		{timestamp: 100},
		{timestamp: 200, sum: optionalHistogramStatistic{true, 0}, min: optionalHistogramStatistic{true, 0}, max: optionalHistogramStatistic{true, 0}},
		{timestamp: 300, sum: optionalHistogramStatistic{true, -2.5}, min: optionalHistogramStatistic{true, -3}, max: optionalHistogramStatistic{true, 4.5}},
		{timestamp: 400, sum: optionalHistogramStatistic{true, 7}},
		{timestamp: 500, min: optionalHistogramStatistic{true, -1}},
		{timestamp: 600, max: optionalHistogramStatistic{true, 9}},
		{timestamp: 700, sum: optionalHistogramStatistic{true, math.Copysign(0, -1)}, min: optionalHistogramStatistic{true, math.Float64frombits(0x7ff8000000000001)}, max: optionalHistogramStatistic{true, math.Inf(1)}},
		{timestamp: 800, sum: optionalHistogramStatistic{true, math.Inf(-1)}},
	}

	for _, metricType := range []string{"Histogram", "ExponentialHistogram"} {
		metricType := metricType
		t.Run(metricType, func(t *testing.T) {
			t.Parallel()
			s, ctx := storetest.New(t)
			md := pmetric.NewMetrics()
			metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			metric.SetName("optional-" + metricType)
			var appendPoint func() optionalHistogramDatapoint
			switch metricType {
			case "Histogram":
				histogram := metric.SetEmptyHistogram()
				histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				appendPoint = func() optionalHistogramDatapoint { return histogram.DataPoints().AppendEmpty() }
			case "ExponentialHistogram":
				histogram := metric.SetEmptyExponentialHistogram()
				histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				appendPoint = func() optionalHistogramDatapoint { return histogram.DataPoints().AppendEmpty() }
			}

			for _, fixture := range fixtures {
				point := appendPoint()
				point.SetTimestamp(pcommon.Timestamp(fixture.timestamp))
				if fixture.sum.present {
					point.SetSum(fixture.sum.value)
				}
				if fixture.min.present {
					point.SetMin(fixture.min.value)
				}
				if fixture.max.present {
					point.SetMax(fixture.max.value)
				}
			}

			require.NoError(t, s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, md)
			}))
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				rows, err := db.QueryContext(ctx, `select timestamp::varchar, sum, min, max
					from metric_datapoints order by timestamp`)
				if err != nil {
					return err
				}
				defer rows.Close()
				for _, fixture := range fixtures {
					require.True(t, rows.Next())
					var timestamp string
					var got [3]sql.NullFloat64
					require.NoError(t, rows.Scan(&timestamp, &got[0], &got[1], &got[2]))
					assert.Equal(t, strconv.FormatUint(fixture.timestamp, 10), timestamp)
					for i, want := range []optionalHistogramStatistic{fixture.sum, fixture.min, fixture.max} {
						assert.Equal(t, want.present, got[i].Valid)
						if want.present {
							assert.Equal(t, math.Float64bits(want.value), math.Float64bits(got[i].Float64))
						}
					}
				}
				require.False(t, rows.Next())
				return rows.Err()
			}))

			byTimestamp := make(map[string]map[string]any)
			for _, raw := range metricDatapoints(getMetricFullByName(t, s, ctx, "optional-"+metricType)) {
				dp := raw.(map[string]any)
				byTimestamp[dp["timestamp"].(string)] = dp
			}
			for _, fixture := range fixtures {
				dp := byTimestamp[strconv.FormatUint(fixture.timestamp, 10)]
				require.NotNil(t, dp)
				for field, want := range map[string]optionalHistogramStatistic{"sum": fixture.sum, "min": fixture.min, "max": fixture.max} {
					require.Contains(t, dp, field)
					if want.present {
						if !math.IsNaN(want.value) && !math.IsInf(want.value, 0) && !(want.value == 0 && math.Signbit(want.value)) {
							assert.Equal(t, want.value, dp[field])
						} else {
							assert.Equal(t, fmt.Sprintf("0x%016x", math.Float64bits(want.value)), dp[field])
						}
					} else {
						assert.Nil(t, dp[field])
					}
				}
			}

			receivedByTimestamp := make(map[string]map[string]any)
			for _, raw := range exactMetricSeriesDatapoints(t,
				getExactMetricSeriesByName(t, s, ctx, "optional-"+metricType, store.TimeRange{})) {
				dp := raw.(map[string]any)
				receivedByTimestamp[dp["timestamp"].(string)] = dp
			}
			for _, fixture := range fixtures {
				dp := receivedByTimestamp[strconv.FormatUint(fixture.timestamp, 10)]
				require.NotNil(t, dp)
				for field, want := range map[string]optionalHistogramStatistic{"sum": fixture.sum, "min": fixture.min, "max": fixture.max} {
					if !want.present {
						assert.NotContains(t, dp, field)
						continue
					}
					require.Contains(t, dp, field)
					if !math.IsNaN(want.value) && !math.IsInf(want.value, 0) && !(want.value == 0 && math.Signbit(want.value)) {
						assert.Equal(t, want.value, dp[field])
					} else {
						assert.Equal(t, fmt.Sprintf("0x%016x", math.Float64bits(want.value)), dp[field])
					}
				}
			}
		})
	}
}

func TestHistogramReductionRequiresEveryInputSum(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		metricType  string
		temporality pmetric.AggregationTemporality
	}{
		{name: "delta", metricType: "Histogram", temporality: pmetric.AggregationTemporalityDelta},
		{name: "cumulative", metricType: "ExponentialHistogram", temporality: pmetric.AggregationTemporalityCumulative},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, ctx := storetest.New(t)
			md := pmetric.NewMetrics()
			metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			metric.SetName("partial-" + tc.name)

			switch tc.metricType {
			case "Histogram":
				histogram := metric.SetEmptyHistogram()
				histogram.SetAggregationTemporality(tc.temporality)
				for i := range 3 {
					timestamp := pcommon.Timestamp(time.Duration(i+1) * time.Second)
					count := uint64(i + 1)
					dp := histogram.DataPoints().AppendEmpty()
					dp.SetTimestamp(timestamp)
					dp.SetCount(count)
					dp.BucketCounts().FromRaw([]uint64{count})
					if i != 1 {
						dp.SetSum(float64(count))
					}
				}
			case "ExponentialHistogram":
				histogram := metric.SetEmptyExponentialHistogram()
				histogram.SetAggregationTemporality(tc.temporality)
				for i := range 3 {
					timestamp := pcommon.Timestamp(time.Duration(i+1) * time.Second)
					count := uint64(i + 1)
					dp := histogram.DataPoints().AppendEmpty()
					dp.SetTimestamp(timestamp)
					dp.SetCount(count)
					dp.Positive().BucketCounts().FromRaw([]uint64{count})
					if i != 1 {
						dp.SetSum(float64(count))
					}
				}
			}

			require.NoError(t, s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, md)
			}))
			metricID := findMetricRef(t, s, ctx, "partial-"+tc.name)
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, metricID,
					store.BoundedTimeRange(0, int64(4*time.Second)),
					1, nil, nil, 0, 0, 0, nil, "", nil, 0)

			})
			require.NoError(t, err)
			var full map[string]any
			require.NoError(t, json.Unmarshal(raw, &full))
			dps := metricDatapoints(full)
			require.Len(t, dps, 1)
			assert.Contains(t, dps[0].(map[string]any), "sum")
			assert.Nil(t, dps[0].(map[string]any)["sum"])
			aggregate := full["aggregate"].([]any)
			require.Len(t, aggregate, 1)
			assert.Contains(t, aggregate[0].(map[string]any), "sum")
			assert.Nil(t, aggregate[0].(map[string]any)["sum"])
		})
	}
}

func TestHistogramSelectionIsPartitionedByExactScopeIdentity(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	for i, fixture := range []struct {
		schema string
		build  string
		count  uint64
	}{{"scope-blue", "blue", 3}, {"scope-green", "green", 7}} {
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.SetSchemaUrl(fixture.schema)
		sm.Scope().SetName("otelhttp")
		sm.Scope().SetVersion("1.2.0")
		sm.Scope().Attributes().PutStr("build", fixture.build)
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("http.server.duration")
		metric.SetUnit("ms")
		histogram := metric.SetEmptyHistogram()
		histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dp := histogram.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + i))
		dp.SetCount(fixture.count)
		dp.SetSum(float64(fixture.count))
		dp.ExplicitBounds().FromRaw([]float64{1})
		dp.BucketCounts().FromRaw([]uint64{fixture.count, 0})
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 2)
	wantCounts := map[string]uint64{"scope-blue": 3, "scope-green": 7}
	for _, summary := range summaries {
		assert.EqualValues(t, 1, summary["seriesCount"])
		assert.EqualValues(t, 1, summary["dataPointCount"])
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summary["metricRef"].(string),
				store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		schemaURL := metric["scopeSchemaUrl"].(string)
		want, ok := wantCounts[schemaURL]
		require.True(t, ok)
		timeseries := metric["timeseries"].([]any)
		require.Len(t, timeseries, 1)
		points := timeseries[0].(map[string]any)["datapoints"].([]any)
		require.Len(t, points, 1)
		assert.Equal(t, want, metricWireUint64(t, points[0].(map[string]any)["count"]))
	}
}

// An exponential histogram's zero_threshold T declares that observations at or
// below T live in zero_count rather than in a bucket. Merging datapoints with
// different thresholds takes the larger, because the merged histogram cannot
// claim to resolve values one of its inputs could not. The input with the
// smaller threshold is then contributing buckets that fall entirely below the
// merged threshold, and those counts have to move into zero_count -- otherwise
// zero_threshold says one thing and the bucket array says another.
//
// Worked by hand at scale 0, where base = 2 and bucket i covers (2^i, 2^(i+1)]:
//
//	A: threshold 1, zeroCount 1, counts [5, 7] at offset 0
//	B: threshold 4, zeroCount 2, counts [0, 0, 3] at offset 0
//
//	merged threshold = 4
//	cutoff           = floor(log2(4) * 2^0) - 1 = 1
//	summed counts    = [5, 7, 3] at offset 0
//	folded           = buckets 0 and 1 = 5 + 7 = 12
//	result           = [3] at offset 2, zeroCount 1 + 2 + 12 = 15
func TestExpHistogramMerge_FoldsBucketsBelowMergedZeroThreshold(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("http.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: base, scale: 0,
			zeroCount: 1, zeroThreshold: 1,
			posOffset: 0, posCounts: []uint64{5, 7},
			count: 13, sum: 40,
		},
		{
			timestamp: base.Add(time.Millisecond), scale: 0,
			zeroCount: 2, zeroThreshold: 4,
			posOffset: 0, posCounts: []uint64{0, 0, 3},
			count: 5, sum: 30,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Resolution 1 puts both datapoints in a single bucket, which is what makes
	// them merge at all.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(0,
			time.Now().UnixNano()+int64(time.Hour)),
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1, "one series")
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one")
	dp := dps[0].(map[string]any)

	assert.Equal(t, float64(4), dp["zeroThreshold"], "merged threshold is the larger of the two")
	assert.Equal(t, uint64(15), metricWireUint64(t, dp["zeroCount"]),
		"buckets below the merged threshold must be folded into zero_count")
	assert.Equal(t, float64(2), dp["positiveBucketOffset"],
		"the folded buckets are gone, so the array starts above the cutoff")

	counts, _ := dp["positiveBucketCounts"].([]any)
	require.Len(t, counts, 1)
	assert.Equal(t, uint64(3), metricWireUint64(t, counts[0]), "only the bucket above the threshold survives")

	// Moving counts must conserve observations.
	assert.Equal(t, uint64(18), metricWireUint64(t, dp["count"]), "total observations conserved")
}

// TestCumulativeHistogramMerge_DifferencesAcrossBuckets covers the reduction of
// a cumulative histogram whose buckets hold one datapoint each.
//
// A cumulative reading is a running total, so a bucket's activity is measured
// against the reading before it, which is in the previous bucket whenever
// the requested width is at or below the reporting cadence. Differencing within
// the bucket would report zero activity when it contains one reading.
//
// The scalar path already answers this correctly (scalar_lagged differences
// each datapoint against its predecessor in the series, then buckets); this
// pins the histogram merge to the same rule.
func TestCumulativeHistogramMerge_DifferencesAcrossBuckets(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}

	// Running totals: two more observations every minute, all in the first
	// bucket of the vector. Activity per minute is 2, forever.
	var dps []histTestDP
	for i := range 6 {
		n := uint64(2 * (i + 1))
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{n, 0, 0, 0},
			count:     n,
			sum:       float64(n) * 5,
			min:       5, max: 5,
		})
	}
	fixture := makeHistogramFixtureT("climb.hist", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	get := func(targetBuckets int64) []map[string]any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
				targetBuckets, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		ts := m["timeseries"].([]any)
		require.Len(t, ts, 1)
		var out []map[string]any
		for _, d := range ts[0].(map[string]any)["datapoints"].([]any) {
			out = append(out, d.(map[string]any))
		}
		return out
	}

	// Unreduced: the running totals themselves, untouched.
	unreduced := get(0)
	require.Len(t, unreduced, 6)
	assert.Equal(t, uint64(12), metricWireUint64(t, unreduced[0]["count"]),
		"without a reduction a cumulative datapoint keeps its running total")

	// Reduced onto minute buckets, which is the cadence: every bucket holds one
	// datapoint and must be differenced against the previous bucket's.
	for _, targetBuckets := range []int64{6, 60, 600} {
		merged := get(targetBuckets)
		require.NotEmpty(t, merged, "targetBuckets=%d", targetBuckets)

		// The first reading establishes the baseline and measures no interval,
		// exactly as the scalar path drops a series' first datapoint.
		assert.Len(t, merged, 5,
			"targetBuckets=%d: six readings describe five intervals", targetBuckets)

		var total uint64
		for _, dp := range merged {
			assert.Equal(t, uint64(2), metricWireUint64(t, dp["count"]),
				"targetBuckets=%d: each minute adds two observations", targetBuckets)
			assert.Equal(t, float64(10), dp["sum"],
				"targetBuckets=%d: sum is differenced with count", targetBuckets)
			buckets := dp["bucketCounts"].([]any)
			var vec uint64
			for _, c := range buckets {
				vec += metricWireUint64(t, c)
			}
			assert.Equal(t, metricWireUint64(t, dp["count"]), vec,
				"targetBuckets=%d: the vector agrees with the count on the same row",
				targetBuckets)
			total += metricWireUint64(t, dp["count"])
		}
		assert.Equal(t, uint64(10), total,
			"targetBuckets=%d: the intervals sum to the counter's climb (12-2)",
			targetBuckets)
	}
}

func TestCumulativeHistogramMerge_UnchangedReadingsRetainRecordedCounts(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Date(2026, 10, 20, 13, 0, 0, 0, time.UTC)
	var dps []histTestDP
	for i := range 3 {
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			bounds:    []float64{10}, counts: []uint64{12, 0},
			count: 12, sum: 60, min: 5, max: 5,
		})
	}
	fixture := makeHistogramFixtureT("unchanged.hist", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	metricID := searchMetricsAll(t, s, ctx)[0]["metricRef"].(string)
	for _, targetBuckets := range []int64{0, 1, 100} {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
				targetBuckets, nil, nil, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		points := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		require.NotEmpty(t, points)
		var count uint64
		var sum float64
		if targetBuckets == 0 {
			require.Len(t, points, 3)
			count, sum = 12, 60
		}
		for _, point := range points {
			dp := point.(map[string]any)
			assert.Equal(t, count, metricWireUint64(t, dp["count"]))
			assert.Equal(t, sum, dp["sum"])
			assert.Equal(t, count, metricWireUint64(t, dp["bucketCounts"].([]any)[0]))
		}
	}
}

// TestCumulativeHistogramMerge_ResetIsConsistentAcrossFields covers a counter
// restart inside a merged bucket.
//
// A falling counter means it restarted, so the later reading is activity since
// the reset. Count, sum, and bucket vectors must apply that decision together.
func TestCumulativeHistogramMerge_ResetIsConsistentAcrossFields(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}
	// Climb to 105, restart, climb to 8 -- all inside one merged bucket.
	readings := []uint64{100, 105, 3, 8}
	var dps []histTestDP
	for i, n := range readings {
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Second),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{n, 0, 0, 0},
			count:     n,
			sum:       float64(n),
			min:       1, max: 1,
		})
	}
	fixture := makeHistogramFixtureT("reset.hist", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	dpl := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dpl, 1, "all four readings merge into one bucket")
	dp := dpl[0].(map[string]any)

	// 5 before the restart, then 3 (the reading itself), then 5 after.
	assert.Equal(t, uint64(13), metricWireUint64(t, dp["count"]),
		"activity is summed per reading, with the restart's own value counted once")

	var vec uint64
	for _, c := range dp["bucketCounts"].([]any) {
		vec += metricWireUint64(t, c)
	}
	assert.Equal(t, metricWireUint64(t, dp["count"]), vec,
		"the bucket vector and the count describe the same observations")
	assert.Equal(t, float64(metricWireUint64(t, dp["count"])), dp["sum"],
		"and so does the sum, since every observation here has value 1")
}

func TestHistogramReductionPreservesUnsignedCountDomain(t *testing.T) {
	t.Parallel()
	const maxUint64 = ^uint64(0)
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)

	getReduced := func(t *testing.T, fixture pmetric.Metrics) map[string]any {
		t.Helper()
		s, ctx := storetest.New(t)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, fixture)
		}))
		summaries := searchMetricsAll(t, s, ctx)
		require.Len(t, summaries, 1)
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),
				1, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))
		return got
	}

	t.Run("cumulative explicit histogram", func(t *testing.T) {
		readings := []uint64{0, 1 << 63, maxUint64, 1}
		dps := make([]histTestDP, 0, len(readings))
		for i, count := range readings {
			dps = append(dps, histTestDP{
				timestamp: base.Add(time.Duration(i) * time.Second),
				bounds:    []float64{1},
				counts:    []uint64{count, 0},
				count:     count,
			})
		}
		got := getReduced(t, makeHistogramFixtureT(
			"uint64.explicit", pmetric.AggregationTemporalityCumulative, dps))

		datapoints := metricDatapoints(got)
		require.Len(t, datapoints, 1)
		dp := datapoints[0].(map[string]any)
		assert.Equal(t, "18446744073709551616", dp["count"])
		assert.Equal(t, []any{"18446744073709551616", "0"}, dp["bucketCounts"])

		aggregate := got["aggregate"].([]any)
		require.Len(t, aggregate, 1)
		assert.Equal(t, float64(18446744073709551616), aggregate[0].(map[string]any)["count"])
	})

	t.Run("delta exponential histogram", func(t *testing.T) {
		fixture := makeExpHistogramFixtureT("uint64.exponential", pmetric.AggregationTemporalityDelta,
			[]expHistTestDP{
				{timestamp: base, scale: 1, posCounts: []uint64{maxUint64}, count: maxUint64},
				{timestamp: base.Add(time.Second), scale: 0, posCounts: []uint64{maxUint64}, count: maxUint64},
			})
		got := getReduced(t, fixture)

		datapoints := metricDatapoints(got)
		require.Len(t, datapoints, 1)
		dp := datapoints[0].(map[string]any)
		assert.Equal(t, "36893488147419103230", dp["count"])
		assert.Equal(t, []any{"36893488147419103230"}, dp["positiveBucketCounts"])
		assert.Equal(t, []any{}, dp["negativeBucketCounts"])

		aggregate := got["aggregate"].([]any)
		require.Len(t, aggregate, 1)
		assert.Equal(t, float64(36893488147419103230), aggregate[0].(map[string]any)["count"])
	})
}

// TestGetMetricView_WindowSummaryIsOneBucket verifies that a one-bucket summary
// covers the complete window.
//
// The ladder cannot express it. bucket_width_ns snaps to a nameable width and
// bucketed_dps floors to absolute boundaries, so a window starting mid-rung can
// span several chart buckets. A one-bucket summary bypasses that split.
func TestGetMetricView_WindowSummaryIsOneBucket(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// 10:20 to 12:00 -- a 100 minute span, which the ladder serves with the
	// 1 hour rung, so absolute flooring splits it across 10:00, 11:00 and 12:00.
	base := time.Date(2026, 5, 24, 10, 20, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}
	var dps []histTestDP
	for i := range 21 {
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i*5) * time.Minute),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{1, 1, 0, 0},
			count:     2,
			sum:       25,
			min:       5, max: 15,
		})
	}
	fixture := makeHistogramFixtureT("window.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregateView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			1, nil, []float64{0.5}, 0, 0, nil, "")

	})
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	agg, _ := envelope["aggregate"].([]any)

	require.Len(t, agg, 1,
		"one bucket means one bucket: the caller reads [0] and calls it the window")
	bucket := agg[0].(map[string]any)

	// Every observation, not the share that fell in the first ladder rung.
	assert.Equal(t, float64(42), bucket["count"],
		"21 datapoints of 2 observations each")
	assert.Equal(t, float64(525), bucket["sum"])
	var vec float64
	for _, c := range bucket["bucketCounts"].([]any) {
		vec += c.(float64)
	}
	assert.Equal(t, bucket["count"], vec,
		"the vector describes the same observations as the count")

	// The quantile is the window's, which is the reason a summary cannot be
	// repaired by adding buckets up client-side: quantiles do not sum.
	q, _ := bucket["quantiles"].(map[string]any)
	require.NotNil(t, q)
	assert.NotNil(t, q["0.5"], "a window p50 over every observation in the span")
}

// TestGetMetricView_BoundsMismatchIsReported covers the one histogram disagreement
// that cannot be reconciled: two datapoints in a bucket carrying different
// explicit bounds.
//
// Scales can be reconciled -- downscale_exp_buckets moves both onto the coarser
// one -- but boundaries cannot: there is no transformation that turns [10,20,30]
// into [5,50,500] without inventing observations. So the merge is refused.
//
// The response must report each refused merge so it cannot look like an idle
// interval.
func TestGetMetricView_BoundsMismatchIsReported(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	attrs := map[string]string{"pod": "a"}
	// Same bucket, two boundary sets: the SDK's view was reconfigured between
	// these two readings.
	dps := []histTestDP{
		{
			timestamp: base, attrs: attrs,
			bounds: []float64{10, 20, 30}, counts: []uint64{1, 1, 0, 0},
			count: 2, sum: 25, min: 5, max: 15,
		},
		{
			timestamp: base.Add(time.Second), attrs: attrs,
			bounds: []float64{5, 50, 500}, counts: []uint64{2, 0, 0, 0},
			count: 2, sum: 4, min: 2, max: 2,
		},
	}
	fixture := makeHistogramFixtureT("drift.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	get := func(targetBuckets int64) map[string]any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

				targetBuckets, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}

	// Reduced: the merge is refused, the bucket is gone, and the response says
	// so instead of leaving the reader to notice a hole.
	reduced := get(1)
	mismatch, _ := reduced["boundsMismatch"].(map[string]any)
	require.NotNil(t, mismatch,
		"a refused merge is reported, not merely omitted")
	assert.Equal(t, float64(1), mismatch["seriesBuckets"],
		"one (series, bucket) merge refused")
	assert.Equal(t, float64(1), mismatch["aggregateBuckets"],
		"and the cross-series merge over the same bucket, for the same reason")

	// The rows really are absent -- this is a report of a refusal, not a
	// warning attached to a merged-anyway row. With every bucket refused the
	// series has nothing left to carry, so it drops out of the response
	// entirely; the report is the only thing that says why.
	assert.Empty(t, reduced["timeseries"],
		"a series whose every bucket was refused has nothing to show")
	assert.Nil(t, reduced["aggregate"],
		"and the cross-series merge over those buckets is refused with it, "+
			"rather than summing vectors measured against different boundaries")

	// Unreduced: nothing is merged, so nothing is refused, and the datapoints
	// arrive exactly as they were recorded -- differing bounds and all.
	unreduced := get(0)
	assert.Nil(t, unreduced["boundsMismatch"],
		"no merge, no refusal to report")
	uts := unreduced["timeseries"].([]any)
	require.Len(t, uts, 1)
	assert.Len(t, uts[0].(map[string]any)["datapoints"], 2,
		"both readings survive when nothing is being merged")
}

// TestExpHistogramMerge_RescalesBeforeSumming covers a series whose scale
// drifts mid-flight: an SDK downscales as the observed range widens, so two
// datapoints of the *same* series can carry different scales and offsets.
//
// Summing their bucket vectors positionally would add counts covering
// different value ranges together -- wrong quantiles, no error, no way to see
// it from the chart. The merge has to bring both onto the coarsest scale
// present first.
func TestExpHistogramMerge_RescalesBeforeSumming(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("rescale.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		// Scale 2, four buckets from offset 4.
		{
			timestamp: base, scale: 2,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 4, posCounts: []uint64{1, 1, 1, 1},
			count: 4, sum: 8,
		},
		// Scale 1, two buckets from offset 2 -- the same value range, half the
		// resolution. Downscaling the first by one step lands exactly here,
		// which is what makes the overlap checkable rather than approximate.
		{
			timestamp: base.Add(time.Second), scale: 1,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 2, posCounts: []uint64{5, 5},
			count: 10, sum: 20,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Target 1 puts both datapoints in one bucket, which is what makes them
	// merge at all.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one bucket")
	dp := dps[0].(map[string]any)

	// The merge lands on the coarsest scale present, not the first seen.
	assert.Equal(t, float64(1), dp["scale"],
		"merging must downscale to the coarsest scale in the group")

	// Total count is conserved however the buckets are aligned: 4 + 10.
	counts, _ := dp["positiveBucketCounts"].([]any)
	var total uint64
	for _, c := range counts {
		total += metricWireUint64(t, c)
	}
	assert.Equal(t, uint64(14), total, "no observation invented or lost")
	assert.Equal(t, uint64(14), metricWireUint64(t, dp["count"]), "the reported count agrees with the buckets")

	// The scale-2 datapoint's four buckets at offset 4 collapse to two at
	// offset 2 -- exactly where the scale-1 datapoint already sits. A correct
	// merge overlaps them; a positional sum would lay them side by side.
	assert.Equal(t, float64(2), dp["positiveBucketOffset"])
	require.Len(t, counts, 2, "overlapped, not concatenated")
	assert.Equal(t, uint64(7), metricWireUint64(t, counts[0]))
	assert.Equal(t, uint64(7), metricWireUint64(t, counts[1]))
}

// TestExpHistogramMerge_CumulativeSubtractsAcrossAScaleChange verifies
// cumulative subtraction after aligning datapoints to a common scale.
//
// Each datapoint is a running total, so the activity in a bucket is the last
// minus the first. Vectors at different scales are not directly comparable.
func TestExpHistogramMerge_CumulativeSubtractsAcrossAScaleChange(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("cumulative.duration", pmetric.AggregationTemporalityCumulative, []expHistTestDP{
		// 10 observations so far, at scale 2.
		{
			timestamp: base, scale: 2,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 4, posCounts: []uint64{5, 5, 0, 0},
			count: 10, sum: 10,
		},
		// 30 by the next datapoint, and the SDK downscaled in between. The
		// activity in this bucket is 20, not 30.
		{
			timestamp: base.Add(time.Second), scale: 1,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 2, posCounts: []uint64{15, 15},
			count: 30, sum: 30,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one bucket")
	dp := dps[0].(map[string]any)

	// The number that matters: activity, not the running total.
	assert.Equal(t, uint64(20), metricWireUint64(t, dp["count"]),
		"a cumulative bucket reports the activity within it, not the counter's value")

	counts, _ := dp["positiveBucketCounts"].([]any)
	var total uint64
	for _, c := range counts {
		total += metricWireUint64(t, c)
	}
	assert.Equal(t, uint64(20), total,
		"the bucket vectors must be differenced on a common scale, not passed through")
}

// TestGetMetricView_Quantiles verifies quantiles computed from stored bucket
// vectors.
//
// At scale 0 the buckets are (1,2] count 10, (2,4] count 20, and (4,8]
// count 30, so p50's target of 30 is the upper edge of the second bucket.
//
// Empty means none, so a caller drawing no overlays does not pay for them.
func TestGetMetricView_Quantiles(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("q.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{{
		timestamp: base, scale: 0,
		zeroCount: 0, zeroThreshold: 1,
		posOffset: 0, posCounts: []uint64{10, 20, 30},
		count: 60, sum: 200,
	}})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	datapoint := func(qs []float64) map[string]any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, end), 0, nil, qs, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		ts := m["timeseries"].([]any)
		require.Len(t, ts, 1)
		dps := ts[0].(map[string]any)["datapoints"].([]any)
		require.Len(t, dps, 1)
		return dps[0].(map[string]any)
	}

	withQ := datapoint([]float64{0.5, 0.95, 0.99})
	q, ok := withQ["quantiles"].(map[string]any)
	require.True(t, ok, "quantiles must be an object keyed by the quantile")

	assert.InDelta(t, 4.0, q["0.5"], 1e-9)
	assert.InDelta(t, 7.464263932294459, q["0.95"], 1e-9)
	assert.InDelta(t, 7.889861635946874, q["0.99"], 1e-9)

	assert.Nil(t, datapoint(nil)["quantiles"],
		"no quantiles requested means none computed")
}

// TestGetMetricView_QuantileHandWorked pins the quantile arithmetic to values
// worked out by hand through the complete metric-view query.
func TestGetMetricView_QuantileHandWorked(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Now().Add(-time.Minute)

	histCases := []struct {
		name   string
		counts []uint64
		bounds []float64
		q      float64
		want   *float64
	}{
		// counts cumulative: 0, 50, 100, 100, 100. Total=100, p50 target=50.
		// First bucket with acc >= 50 is bucket 2 (1,2]. Linear interp at
		// fraction 1.0 gives 2.0 (the upper bound).
		{"p50 lands cleanly on a bucket boundary",
			[]uint64{0, 50, 50, 0, 0}, []float64{1, 2, 5, 10}, 0.5, ptrF(2.0)},
		// counts cumulative: 0, 10, 30, 60, 100. p95 target=95. Lands in the
		// unbounded tail (bucket 5), where lo=hi=10.0, so we clamp to the
		// last known bound.
		{"p95 in unbounded tail clamps to last known bound",
			[]uint64{0, 10, 20, 30, 40}, []float64{1, 2, 5, 10}, 0.95, ptrF(10.0)},
		// No bounds at all: nothing to interpolate against, so the requested
		// key is present and null rather than absent or an error.
		{"no bounds yields a null quantile",
			[]uint64{10}, nil, 0.5, nil},
		// A populated layout with nothing observed: same null, via the
		// all-zero guard rather than the missing-bounds one.
		{"all-zero counts yield a null quantile",
			[]uint64{0, 0, 0, 0, 0}, []float64{1, 2, 5, 10}, 0.5, nil},
	}
	expCases := []struct {
		name      string
		scale     int32
		zeroCount uint64
		posOffset int32
		posCounts []uint64
		negOffset int32
		negCounts []uint64
		q         float64
		want      *float64
		delta     float64
	}{
		// scale=0 -> base=2. pos_counts=[50,50] at offset=0: bucket1 (1,2]
		// cnt=50, bucket2 (2,4] cnt=50. Total=100. p50 target=50 -> first
		// bucket at acc>=50 is bucket1. loglin: 1 * (2/1)^(50/50) = 2.0.
		{"positive-only p50 (scale=0, two equal buckets)",
			0, 0, 0, []uint64{50, 50}, 0, nil, 0.5, ptrF(2.0), 1e-9},
		// All weight in zero bucket. p50 -> zero bucket -> 0.
		{"zero-only p50 returns 0",
			0, 100, 0, nil, 0, nil, 0.5, ptrF(0.0), 1e-9},
		// neg=[10,10], zero=20, pos=[10,10] at scale=0. Total=60. CDF acc:
		// 10, 20, 40, 50, 60. p50 target=30 -> first acc>=30 is the zero
		// bucket (acc=40). Loglin over [0,0] falls back to linear -> 0.
		{"symmetric three-region p50 lands in zero bucket",
			0, 20, 0, []uint64{10, 10}, 0, []uint64{10, 10}, 0.5, ptrF(0.0), 1e-9},
		// Reference dataset hand-computed in the planning notes. scale=2 ->
		// base = 2^(2^-2) = 2^0.25 ~= 1.189. Hand calc predicted p95 ~= 6.35.
		{"reference dataset p95 matches hand calc",
			2, 0, 6, []uint64{1200, 3800, 4200, 2100, 720, 280, 70, 22, 6, 2}, 0, nil,
			0.95, ptrF(6.349604207872798), 1e-6},
		// scale=0 -> buckets (1,2], (2,4], (4,8]. No zero region, so the
		// smallest observation is above 1: q=0 returns the first populated
		// bucket's lower edge, not 0.
		{"q=0 returns the first populated bucket's lower edge",
			0, 0, 0, []uint64{10, 20, 30}, 0, nil, 0.0, ptrF(1.0), 1e-9},
		// A genuine zero region: the zero bucket is populated, so it is
		// selected on its merits and 0 is the right answer.
		{"q=0 with a real zero region still returns 0",
			0, 5, 0, []uint64{10, 20, 30}, 0, nil, 0.0, ptrF(0.0), 1e-9},
		// An empty bucket in the middle must not absorb the target.
		{"empty interior bucket is skipped",
			0, 0, 0, []uint64{10, 0, 30}, 0, nil, 0.25, ptrF(2.0), 1e-9},
		// Unchanged by the filter -- guards the "no-op for q > 0" claim: the
		// cnt > 0 pick rule must skip only *empty* buckets, never shift a
		// quantile that lands past one.
		{"p50 unaffected by the empty-bucket skip",
			0, 0, 0, []uint64{10, 20, 30}, 0, nil, 0.5, ptrF(4.0), 1e-9},
		// Nothing observed anywhere: the key is present and null.
		{"empty exponential histogram yields a null quantile",
			0, 0, 0, nil, 0, nil, 0.5, nil, 0},
	}

	// One metric per case; each reads its single datapoint's quantile object
	// off the wire.
	for i, tc := range histCases {
		name := fmt.Sprintf("hw.hist.%d", i)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			var total uint64
			for _, c := range tc.counts {
				total += c
			}
			return metrics.Ingest(ctx, conn,
				makeHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, []histTestDP{{
					timestamp: base, bounds: tc.bounds, counts: tc.counts,
					count: total, sum: float64(total),
				}}))
		}))
	}
	for i, tc := range expCases {
		name := fmt.Sprintf("hw.exp.%d", i)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeExpHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, []expHistTestDP{{
					timestamp: base, scale: tc.scale,
					zeroCount: tc.zeroCount, zeroThreshold: 0,
					posOffset: tc.posOffset, posCounts: tc.posCounts,
					negOffset: tc.negOffset, negCounts: tc.negCounts,
				}}))
		}))
	}

	end := time.Now().UnixNano() + int64(time.Hour)
	// A quantile that cannot be computed does not arrive as a null-valued
	// key. metric_datapoint_view_json uses json_merge_patch, whose RFC 7386
	// semantics remove null members.
	quantileOf := func(name string, q float64) (any, bool) {
		id := findMetricRef(t, s, ctx, name)
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(0, end), 0, nil, []float64{q}, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		require.Len(t, dps, 1)
		qobj, ok := dps[0].(map[string]any)["quantiles"].(map[string]any)
		require.True(t, ok, "quantiles must be an object even when empty")
		// Each call requests exactly one quantile, so read the object's only
		// entry rather than reconstruct its key -- DuckDB spells the key from
		// the double (0.0 -> "0.0") where Go's FormatFloat would say "0".
		require.LessOrEqual(t, len(qobj), 1)
		for _, v := range qobj {
			return v, true
		}
		return nil, false
	}

	for i, tc := range histCases {
		t.Run(tc.name, func(t *testing.T) {
			v, present := quantileOf(fmt.Sprintf("hw.hist.%d", i), tc.q)
			if tc.want == nil {
				assert.False(t, present, "an uncomputable quantile is stripped, not null")
			} else {
				require.True(t, present)
				assert.InDelta(t, *tc.want, v, 1e-9)
			}
		})
	}
	for i, tc := range expCases {
		t.Run(tc.name, func(t *testing.T) {
			v, present := quantileOf(fmt.Sprintf("hw.exp.%d", i), tc.q)
			if tc.want == nil {
				assert.False(t, present, "an uncomputable quantile is stripped, not null")
			} else {
				require.True(t, present)
				assert.InDelta(t, *tc.want, v, tc.delta)
			}
		})
	}

	// End-to-end merge, from schema_test's sum_bucket_vectors suite: two
	// series over bounds [1, 2, 5, 10]:
	//   A counts = [0, 50,  50,  0, 0]
	//   B counts = [0, 30,  50, 20, 0]
	//   sum     = [0, 80, 100, 20, 0]  total = 200
	// p50 target = 100. CDF acc = 0, 80, 180, 200, 200. First acc >= 100 is
	// bucket 3 (lo=2, hi=5, cnt=100, acc_prev=80). Linear interp:
	//   2 + (5 - 2) * (100 - 80) / 100 = 2.6
	// map() rejects duplicate keys, so the request is deduplicated first.
	t.Run("duplicate quantiles are tolerated", func(t *testing.T) {
		id := findMetricRef(t, s, ctx, "hw.exp.0")
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(0, end), 0, nil,
				[]float64{0.5, 0.5, 0.95, 0.5}, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err, "a duplicated quantile must not fail the request")
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		qobj := dps[0].(map[string]any)["quantiles"].(map[string]any)
		assert.Len(t, qobj, 2, "duplicates collapse; distinct quantiles survive")
		assert.InDelta(t, 2.0, qobj["0.5"], 1e-9)
	})

	t.Run("aggregate p50 over merged series", func(t *testing.T) {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeHistogramFixtureT("hw.merge", pmetric.AggregationTemporalityDelta, []histTestDP{
					{timestamp: base, attrs: map[string]string{"route": "/a"},
						bounds: []float64{1, 2, 5, 10}, counts: []uint64{0, 50, 50, 0, 0}, count: 100, sum: 100},
					{timestamp: base, attrs: map[string]string{"route": "/b"},
						bounds: []float64{1, 2, 5, 10}, counts: []uint64{0, 30, 50, 20, 0}, count: 100, sum: 100},
				}))
		}))
		id := findMetricRef(t, s, ctx, "hw.merge")
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricAggregateView(ctx, db, id, store.BoundedTimeRange(0, end), 1, nil, []float64{0.5}, 0, 1, nil, "")
		})
		require.NoError(t, err)
		var env map[string]any
		require.NoError(t, json.Unmarshal(raw, &env))
		agg := env["aggregate"].([]any)
		require.Len(t, agg, 1)
		q := agg[0].(map[string]any)["quantiles"].(map[string]any)
		assert.InDelta(t, 2.6, q["0.5"], 1e-9)
	})
}

// TestGetMetricView_CrossSeriesAggregate covers the merge that lets the client stop
// combining series itself.
//
// Two series, different scales, in one time bucket. The aggregate must put them
// on a common scale, align offsets and sum -- conserving every observation,
// which is the property a merge can quietly break and a chart will never show.
//
// Worked by hand. Scale 0 has base 2 and bucket i covering (2^i, 2^(i+1)];
// scale -1 has base 4 and bucket j covering (4^j, 4^(j+1)]. The coarser of the
// two wins, so the scale-0 series is downscaled by one step, folding its
// buckets in pairs:
//
//	A scale  0, counts [10, 20, 30] at offset 0  -> [30, 30] at offset 0
//	B scale -1, counts [5, 5]       at offset 0
//	merged   scale -1, counts [35, 35] at offset 0
//
// Totals: A has 60 observations, B has 10, so the aggregate must report 70.
func TestGetMetricView_CrossSeriesAggregate(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("agg.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: base, attrs: map[string]string{"route": "/a"},
			scale: 0, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{10, 20, 30},
			count: 60, sum: 100,
		},
		{
			timestamp: base.Add(time.Second), attrs: map[string]string{"route": "/b"},
			scale: -1, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{5, 5},
			count: 10, sum: 20,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(0,
			time.Now().UnixNano()+int64(time.Hour)),
			1, nil, []float64{0.5}, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	require.Len(t, m["timeseries"].([]any), 2, "per-series data still returned")

	agg, ok := m["aggregate"].([]any)
	require.True(t, ok, "aggregate must be present for a histogram merge")
	require.Len(t, agg, 1, "both series fall in one time bucket")
	a := agg[0].(map[string]any)

	assert.Equal(t, float64(-1), a["scale"], "merged on the coarser scale")
	assert.Equal(t, float64(70), a["count"], "every observation conserved")
	assert.Equal(t, float64(0), a["positiveBucketOffset"])

	counts, _ := a["positiveBucketCounts"].([]any)
	require.Len(t, counts, 2)
	assert.Equal(t, float64(35), counts[0], "10+20 downscaled, plus 5")
	assert.Equal(t, float64(35), counts[1], "30 downscaled, plus 5")

	// Bucket counts and the reported total have to tell the same story.
	var sum float64
	for _, c := range counts {
		sum += c.(float64)
	}
	assert.Equal(t, a["count"], sum+a["zeroCount"].(float64),
		"bucket counts plus zero count must equal the reported total")

	q, ok := a["quantiles"].(map[string]any)
	require.True(t, ok, "the aggregate carries quantiles for the summary panel")
	assert.NotNil(t, q["0.5"])

	// min and max are derived from the buckets, because a merge cannot carry
	// them through -- for cumulative it is a subtraction, and two minima do not
	// subtract into the minimum of what happened between them. At scale -1 the
	// surviving buckets are (1,4] and (4,16].
	assert.InDelta(t, 1.0, a["min"], 1e-9, "lower edge of the first populated bucket")
	assert.InDelta(t, 16.0, a["max"], 1e-9, "upper edge of the last populated bucket")
}

// TestGetMetricView_EmptyExplicitBoundsAggregate covers the valid explicit
// histogram shape with no boundaries and one catch-all bucket. Representation
// is identified by the presence of explicitBounds, not by whether it contains
// an element: treating [] as exponential drops the only bucket vector from the
// heatmap aggregate even though its top-level count still looks plausible.
func TestGetMetricView_EmptyExplicitBoundsAggregate(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeHistogramFixtureT("agg.catch-all", pmetric.AggregationTemporalityDelta, []histTestDP{
		{
			timestamp: base, attrs: map[string]string{"route": "/a"},
			bounds: nil, counts: []uint64{7}, count: 7, sum: 3.5,
		},
		{
			timestamp: base.Add(time.Second), attrs: map[string]string{"route": "/b"},
			bounds: nil, counts: []uint64{5}, count: 5, sum: 2.5,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	metricID := findMetricRef(t, s, ctx, "agg.catch-all")
	end := base.Add(time.Hour).UnixNano()
	// Poison the inactive exponential columns. Presence of explicit_bounds,
	// including an empty list, must keep aggregate classification explicit.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `update metric_records set datapoints=list_transform(datapoints,lambda d:struct_update(d,
 scale:=0,zero_count:=0,zero_threshold:=0,positive_bucket_offset:=0,positive_bucket_counts:=[d.count],negative_bucket_offset:=0,negative_bucket_counts:=[])) where id=?::uuid`, metricID)
		return err
	}))

	assertCatchAll := func(a map[string]any) {
		t.Helper()
		require.Contains(t, a, "explicitBounds")
		require.IsType(t, []any{}, a["explicitBounds"])
		assert.Empty(t, a["explicitBounds"])
		assert.Equal(t, []any{float64(12)}, a["bucketCounts"])
		assert.Equal(t, float64(12), a["count"])
		assert.NotContains(t, a, "min")
		assert.NotContains(t, a, "max")
		assert.NotContains(t, a, "scale")
		assert.NotContains(t, a, "positiveBucketCounts")
		quantiles, ok := a["quantiles"].(map[string]any)
		require.True(t, ok,
			"requesting a quantile must exercise the aggregate quantile classification")
		require.Contains(t, quantiles, "0.5")
		assert.Nil(t, quantiles["0.5"],
			"a catch-all bucket without finite boundaries has no computable quantile")
	}

	full, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, end), 1, nil,
			[]float64{0.5}, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(full, &metric))

	series := metric["timeseries"].([]any)
	require.Len(t, series, 2)
	var seriesBucketTotal uint64
	for _, rawSeries := range series {
		dps := rawSeries.(map[string]any)["datapoints"].([]any)
		require.Len(t, dps, 1, "a whole-window request merges each series once")
		dp := dps[0].(map[string]any)
		require.IsType(t, []any{}, dp["explicitBounds"])
		assert.Empty(t, dp["explicitBounds"])
		assert.Contains(t, dp, "min")
		assert.Nil(t, dp["min"])
		assert.Contains(t, dp, "max")
		assert.Nil(t, dp["max"])
		counts := dp["bucketCounts"].([]any)
		require.Len(t, counts, 1)
		seriesBucketTotal += metricWireUint64(t, counts[0])
	}
	assert.Equal(t, uint64(12), seriesBucketTotal,
		"whole-window per-series histograms must retain both catch-all buckets")

	aggregate := metric["aggregate"].([]any)
	require.Len(t, aggregate, 1)
	assertCatchAll(aggregate[0].(map[string]any))

	only, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregateView(ctx, db, metricID, store.BoundedTimeRange(0, end), 1, nil,
			[]float64{0.5}, 0, 0, nil, "")

	})
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(only, &envelope))
	onlyAggregate := envelope["aggregate"].([]any)
	require.Len(t, onlyAggregate, 1)
	assertCatchAll(onlyAggregate[0].(map[string]any))
}

// TestGetMetricView_MergedRowsCarryTheirBucket verifies that merged histogram
// rows use their common bucket start rather than a constituent timestamp.
//
// The window is explicit and the target chosen so the ladder in bucket_width_ns
// lands on exactly 10s: 60s / 6 admits 10s and refuses 5s. Pinning the width is
// what lets the assertion be "on the grid" rather than "self-consistent".
func TestGetMetricView_MergedRowsCarryTheirBucket(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// 13:00:00 UTC is a whole minute, so it is already on the 10s grid and the
	// expected bucket starts are base, base+10s, base+20s exactly.
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	at := func(d time.Duration, driver string) histTestDP {
		return histTestDP{
			timestamp: base.Add(d),
			attrs:     map[string]string{"driver": driver},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	// Two series, several datapoints per bucket, none of them landing on a
	// boundary -- so any surviving constituent timestamp shows up immediately.
	fixture := makeHistogramFixtureT("bucket.duration", pmetric.AggregationTemporalityDelta, []histTestDP{
		at(1*time.Second, "ALO"),
		at(7*time.Second, "ALO"),
		at(11*time.Second, "ALO"),
		at(3*time.Second, "VER"),
		at(25*time.Second, "VER"),
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(
			base.UnixNano(), base.Add(60*time.Second).UnixNano()),

			6, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	const widthNs = int64(10 * time.Second)
	bucketOf := func(d time.Duration) int64 { return base.Add(d).UnixNano() }

	series, _ := m["timeseries"].([]any)
	require.NotEmpty(t, series)

	perSeries := map[string][]int64{}
	for _, s := range series {
		ts := s.(map[string]any)
		key := ts["seriesRef"].(string)
		for _, d := range ts["datapoints"].([]any) {
			v, err := strconv.ParseInt(d.(map[string]any)["timestamp"].(string), 10, 64)
			require.NoError(t, err)
			perSeries[key] = append(perSeries[key], v)
		}
	}

	var all []int64
	for _, stamps := range perSeries {
		for _, v := range stamps {
			all = append(all, v)
			assert.Zero(t, v%widthNs,
				"a merged row must sit on the bucket grid, not on a constituent's timestamp")
		}
	}
	require.NotEmpty(t, all)

	// The exact buckets, so "on the grid" cannot be satisfied by rounding to
	// the wrong one. Bucket 0 holds datapoints from both series.
	distinct := map[int64]bool{}
	for _, v := range all {
		distinct[v] = true
	}
	assert.Equal(t, map[int64]bool{
		bucketOf(0): true, bucketOf(10 * time.Second): true, bucketOf(20 * time.Second): true,
	}, distinct)

	// Both series merged over bucket 0 must name it identically -- the point of
	// the grid is that rows from different series line up on it.
	require.Len(t, perSeries, 2)
	for key, stamps := range perSeries {
		assert.Contains(t, stamps, bucketOf(0), "series %s must report bucket 0 by its start", key)
	}

	// The aggregate is the same merge across series and has to agree, or the
	// heatmap's columns and the rows beneath them sit on different clocks.
	for _, b := range m["aggregate"].([]any) {
		v, err := strconv.ParseInt(b.(map[string]any)["timestamp"].(string), 10, 64)
		require.NoError(t, err)
		assert.Zero(t, v%widthNs, "an aggregate bucket must sit on the grid too")
	}
}

// TestGetMetricAggregateView covers the aggregate-only call, which exists so a
// legend toggle does not re-ship the per-series payload.
//
// It must answer the same question as GetMetricView's aggregate field for the same
// arguments -- it runs the same query and keeps one field, and this is what
// stops those two drifting.
func TestGetMetricAggregateView(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().Add(-time.Minute)
	var dps []expHistTestDP
	for si := 0; si < 3; si++ {
		dps = append(dps, expHistTestDP{
			timestamp: base.Add(time.Duration(si) * time.Second),
			attrs:     map[string]string{"route": fmt.Sprintf("/r%d", si)},
			scale:     0, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{2, 3},
			count: 5, sum: 10,
		})
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn,
			makeExpHistogramFixtureT("agg.only", pmetric.AggregationTemporalityDelta, dps))
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	// Same arguments to both calls.
	full, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, end), 1, nil, []float64{0.5}, 0, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(full, &m))
	fromFull, _ := json.Marshal(m["aggregate"])

	only, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregateView(ctx, db, metricID, store.BoundedTimeRange(0, end), 1, nil, []float64{0.5}, 0, 0, nil, "")
	})
	require.NoError(t, err)

	// One endpoint carries the histogram merge and scalar pools for both metric
	// shapes.
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(only, &envelope))
	fromOnly, _ := json.Marshal(envelope["aggregate"])
	assert.JSONEq(t, string(fromFull), string(fromOnly),
		"the aggregate-only call must match GetMetricView's aggregate field")

	// A histogram has no scalar pools: scalar_dps is Gauge and Sum only, so
	// nothing reaches the fold.
	scalar, _ := envelope["scalarAggregate"].(map[string]any)
	require.NotNil(t, scalar, "the field is present even when it is empty")
	assert.Empty(t, scalar["all"], "a histogram contributes no scalar pool")
	assert.Empty(t, scalar["selected"])

	// And it honours the selection, which is the reason it exists.
	var agg []map[string]any
	fromOnlyRaw, _ := json.Marshal(envelope["aggregate"])
	require.NoError(t, json.Unmarshal(fromOnlyRaw, &agg))
	require.NotEmpty(t, agg)
	allCount := agg[0]["count"].(float64)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregateView(ctx, db, metricID, store.BoundedTimeRange(0, end), 1, []string{}, nil, 0, 0, nil, "")
	})
	require.NoError(t, err)
	var emptyEnvelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &emptyEnvelope))
	assert.Nil(t, emptyEnvelope["aggregate"],
		"an empty selection aggregates no series")

	assert.Positive(t, allCount, "nil selection aggregates every series")

	// The exact SQL projection controls which CTE chains DuckDB can prune.
	assert.ElementsMatch(t, []string{"aggregate", "scalarAggregate"},
		mapKeys(envelope),
		"the aggregate call must project exactly its envelope")

	// A scalar is the mirror image, and the reason the shape is chosen per
	// metric: no histogram merge to report, but real pools.
	t.Run("scalar metric reports pools and no merge", func(t *testing.T) {
		var sums []sumTestDP
		for i := 0; i < 4; i++ {
			sums = append(sums, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Second),
				value:     float64(10 * (i + 1)),
				attrs:     map[string]string{"route": "/scalar"},
			})
		}
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeSumFixtureT("agg.scalar", pmetric.AggregationTemporalityCumulative, sums))
		}))
		scalarID := findMetricRef(t, s, ctx, "agg.scalar")

		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricAggregateView(ctx, db, scalarID, store.BoundedTimeRange(0, end), 1, nil, nil, 0, 4, nil, "")
		})
		require.NoError(t, err)
		var env map[string]any
		require.NoError(t, json.Unmarshal(raw, &env))

		assert.ElementsMatch(t, []string{"aggregate", "scalarAggregate"}, mapKeys(env))
		// Null, not empty: a Sum has no bucket vectors, so there is no merge to
		// report, and the client tells that apart from "merged to nothing".
		assert.Nil(t, env["aggregate"], "a scalar has no histogram merge")
		pools, _ := env["scalarAggregate"].(map[string]any)
		require.NotNil(t, pools)
		assert.NotEmpty(t, pools["all"], "a scalar contributes an All pool")
	})

	// The shared template must leave the full projection intact.
	t.Run("the full projection is unaffected", func(t *testing.T) {
		m := getMetricFullByName(t, s, ctx, "agg.only")
		for _, k := range []string{
			"metricRef", "name", "description", "unit", "metricType", "resource",
			"scope", "timeseries", "aggregate", "scalarAggregate",
			"lastSeenNs", "datapointCount", "window",
		} {
			assert.Containsf(t, m, k, "getMetric must still emit %q", k)
		}
	})
}

// TestGetMetricView_HistogramMergeFollowsTheZoneAcrossDST verifies timezone
// conversion in histogram datapoint bucketing, which scalar view tests do not
// exercise.
func TestGetMetricView_HistogramMergeFollowsTheZoneAcrossDST(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// The same three moments as the scalar test: two on London's 25-hour local
	// Sunday -- one while BST holds, one after the clocks went back -- and one
	// the following Monday.
	morning := time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)
	evening := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	dp := func(ts time.Time) histTestDP {
		return histTestDP{
			timestamp: ts,
			attrs:     map[string]string{"pod": "a"},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	fixture := makeHistogramFixtureT("dst.hist", pmetric.AggregationTemporalityDelta,
		[]histTestDP{dp(morning), dp(evening), dp(monday)})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Two target buckets over ~36 hours of data selects the one-day rung, and
	// a histogram reduction merges rather than elects: each local day's rows
	// fold into one row stamped with the bucket's start.
	shape := func(tzName string, tzOffsetNs int64) []string {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
				2, nil, nil, tzOffsetNs, 0, 0, nil, tzName, nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		var out []string
		for _, v := range dps {
			b := v.(map[string]any)
			out = append(out, fmt.Sprintf("%s/%d",
				b["timestamp"].(string), metricWireUint64(t, b["count"])))
		}
		sort.Strings(out)
		return out
	}
	utc := func(day, hour int) string {
		return strconv.FormatInt(
			time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC).UnixNano(), 10)
	}
	const bstOffsetNs = int64(time.Hour)

	assert.Equal(t,
		[]string{utc(24, 23) + "/2", utc(26, 0) + "/1"},
		shape("Europe/London", bstOffsetNs),
		"the two Sunday observations merge into the Sunday bucket and the "+
			"Monday one stands alone, each stamped with its local midnight")

	assert.Equal(t,
		[]string{utc(24, 23) + "/1", utc(25, 23) + "/2"},
		shape("", bstOffsetNs),
		"the single offset ends the Sunday an hour early, carrying its second "+
			"observation into the Monday -- the behaviour zone-less callers keep")
}

// TestGetMetricView_ColumnWindowMergesTheWholeColumn pins the contract a heatmap
// click depends on: asked for one bucket over one column's time range, the store
// returns each series merged across that whole range.
//
// A heatmap column may contain several per-series buckets even when both grids
// share ladder boundaries. The fixture places fast readings at each column
// start and slow readings afterward so partial-column results are visible.
func TestGetMetricView_ColumnWindowMergesTheWholeColumn(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// Enough readings that the heatmap's target is what sets its width. Fewer and
	// the cadence floor clamps both grids to the 10s reporting interval, they come
	// out identical, and there is no mismatch left to test: the finer grid can
	// never be finer than the data reports.
	const readings = 240
	var dps []histTestDP
	for i := range readings {
		// One in three readings is fast; the rest are slow. With 10s readings and
		// a 30s column, that puts the fast one exactly at each column start.
		fast := i%3 == 0
		counts := []uint64{100, 0, 0, 0}
		sum, mn, mx := 50.0, 0.4, 0.9
		if !fast {
			counts = []uint64{0, 0, 0, 100}
			sum, mn, mx = 5000.0, 40.0, 90.0
		}
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * 10 * time.Second),
			attrs:     map[string]string{"pod": "a"},
			bounds:    []float64{1, 5, 10},
			counts:    counts,
			count:     100,
			sum:       sum, min: mn, max: mx,
		})
	}
	fixture := makeHistogramFixtureT("column.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	id := summaries[0]["metricRef"].(string)

	// The wide read is unbounded, while the column read requests concrete bounds.
	p95 := func(target int64, timeRange store.TimeRange, atTimestamp *int64) float64 {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, timeRange,
				target, nil, []float64{0.95}, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		for _, d := range m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any) {
			dp := d.(map[string]any)
			if atTimestamp != nil {
				v, err := strconv.ParseInt(dp["timestamp"].(string), 10, 64)
				require.NoError(t, err)
				if v != *atTimestamp {
					continue
				}
			}
			q, ok := dp["quantiles"].(map[string]any)
			require.True(t, ok, "quantiles must be present")
			return q["0.95"].(float64)
		}
		t.Fatal("no datapoint matched")
		return 0
	}

	// Find a column boundary the way the UI does: the heatmap's own grid.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, id, store.TimeRange{},
			100, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var heat map[string]any
	require.NoError(t, json.Unmarshal(raw, &heat))
	columns := heat["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Greater(t, len(columns), 2, "need several columns to measure a width")
	first, err := strconv.ParseInt(columns[0].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)
	second, err := strconv.ParseInt(columns[1].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)
	columnWidth := first - second // datapoints arrive newest first
	if columnWidth < 0 {
		columnWidth = -columnWidth
	}
	require.Positive(t, columnWidth)

	// A column in the middle, away from the window edges.
	columnStart, err := strconv.ParseInt(
		columns[len(columns)/2].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)

	// The column's range, half-open. The store's window filter is inclusive at
	// both ends (`timestamp >= start and timestamp <= end`) while floor_div cuts
	// buckets half-open, so asking for [start, start+width] pulls in the next
	// column's first reading and counts it twice -- once in each column. One
	// nanosecond short of the next boundary is the range the column actually
	// covers.
	columnEnd := columnStart + columnWidth - 1

	// Read only the per-series bucket stamped at the column start.
	atStart := p95(500, store.TimeRange{}, &columnStart)
	// What the column holds: one bucket over exactly the column's range, fetched
	// the way the page fetches it.
	whole := p95(1, store.BoundedTimeRange(columnStart, columnEnd), nil)

	assert.NotEqual(t, atStart, whole,
		"a per-series bucket at the column start is not the column: if these agree "+
			"the fixture no longer distinguishes the two reads and the test proves nothing")
	assert.Greater(t, whole, atStart,
		"the column contains the slow readings its first bucket misses, so its p95 "+
			"must be the higher of the two -- this is the number a click has to show")

	// And the window must be exactly one column. Without this the test passes for
	// any window at least a column wide, so a fix that fetched too much would
	// look correct: the p95 would still be the high one, just drawn from readings
	// outside the column the user clicked.
	perColumn := columnWidth / (10 * int64(time.Second)) // readings inside one column
	require.Positive(t, perColumn)
	raw, err = storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(columnStart, columnEnd),
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var merged map[string]any
	require.NoError(t, json.Unmarshal(raw, &merged))
	mergedDps := merged["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, mergedDps, 1, "one bucket over one column is one merged datapoint")
	assert.Equal(t, uint64(perColumn*100), metricWireUint64(t, mergedDps[0].(map[string]any)["count"]),
		"the merged column must total exactly the readings inside it -- %d readings "+
			"of 100 observations each; more means the fetched window is wider than "+
			"the column", perColumn)
}

// Each point retains the bounds received with it, including reconfiguration.
func TestHistogramPointsOwnTheirBounds(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	boundsA := []float64{1, 5, 10}
	boundsB := []float64{2, 4, 8, 16}
	var dps []histTestDP
	for i := range 40 {
		b := boundsA
		counts := []uint64{1, 2, 3, 4}
		if i >= 30 {
			// The SDK was reconfigured after 30 datapoints.
			b = boundsB
			counts = []uint64{1, 2, 3, 4, 5}
		}
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * 10 * time.Second),
			attrs:     map[string]string{"pod": "a"},
			bounds:    b,
			counts:    counts,
			count:     10, sum: 30, min: 0.5, max: 9.5,
		})
	}
	fixture := makeHistogramFixtureT("bounds.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, `select count(distinct explicit_bounds) from metric_datapoints`))
	assert.Equal(t, 40, storetest.CountRows(t, s, ctx, `select count(*) from metric_datapoints where explicit_bounds is not null`))

	// The wire is unchanged: each datapoint comes back with the exact vector
	// it arrived with, read from the owning point.
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	seen := map[int]int{}
	for _, ts := range got["timeseries"].([]any) {
		for _, d := range ts.(map[string]any)["datapoints"].([]any) {
			eb := d.(map[string]any)["explicitBounds"].([]any)
			seen[len(eb)]++
		}
	}
	assert.Equal(t, map[int]int{3: 30, 4: 10}, seen,
		"each datapoint resolves its own vector -- 30 with the original bounds, "+
			"10 with the reconfigured ones")

	// Clearing the owning records removes bounds immediately.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	}))
	assert.Zero(t, storetest.CountRows(t, s, ctx, `select count(*) from metric_records`))

	// Reingestion restores every point's original bounds without a cache.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, makeHistogramFixtureT("bounds.hist",
			pmetric.AggregationTemporalityDelta, dps))
	}))
	assert.Equal(t, 2, storetest.CountRows(t, s, ctx, `select count(distinct explicit_bounds) from metric_datapoints`))
}

// collector stores metrics decoded by pdata's OTLP gRPC server.
type collector struct {
	pmetricotlp.UnimplementedGRPCServer
	batches []pmetric.Metrics
}

func (c *collector) Export(_ context.Context, req pmetricotlp.ExportRequest) (pmetricotlp.ExportResponse, error) {
	md := pmetric.NewMetrics()
	req.Metrics().CopyTo(md)
	c.batches = append(c.batches, md)
	return pmetricotlp.NewExportResponse(), nil
}

// TestCumulativeMergeAgainstTheOtelSDK verifies cumulative merging against
// running totals produced by the OpenTelemetry Go SDK. Only observations from
// the second collection contribute to the expected window delta.
func TestCumulativeMergeAgainstTheOtelSDK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	sink := &collector{}
	srv := grpc.NewServer()
	pmetricotlp.RegisterGRPCServer(srv, sink)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(lis.Addr().String()),
		otlpmetricgrpc.WithInsecure(),
		// Explicit, though it is also the default: this test is about cumulative.
		otlpmetricgrpc.WithTemporalitySelector(func(sdkmetric.InstrumentKind) metricdata.Temporality {
			return metricdata.CumulativeTemporality
		}),
	)
	require.NoError(t, err)

	reader := sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(time.Hour))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	bounds := []float64{1, 5, 10}
	hist, err := provider.Meter("sdk-cumulative-test").Float64Histogram(
		"request.duration",
		otelmetric.WithExplicitBucketBoundaries(bounds...),
	)
	require.NoError(t, err)
	attrs := otelmetric.WithAttributes(attribute.String("route", "/checkout"))

	// The first collection establishes the cumulative baseline.
	firstCycle := []float64{0.5, 0.7, 2.0}
	for _, v := range firstCycle {
		hist.Record(ctx, v, attrs)
	}
	require.NoError(t, provider.ForceFlush(ctx))

	// The second collection covers every explicit bucket and the overflow bucket.
	secondCycle := []float64{0.5, 3.0, 7.0, 7.5, 50.0}
	for _, v := range secondCycle {
		hist.Record(ctx, v, attrs)
	}
	require.NoError(t, provider.ForceFlush(ctx))
	require.NoError(t, provider.Shutdown(ctx))

	// Shutdown may emit an unchanged cumulative batch after the forced collections.
	require.GreaterOrEqual(t, len(sink.batches), 2, "one batch per collection")

	s, storeCtx := storetest.New(t)
	for _, md := range sink.batches {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(storeCtx, conn, md)
		}))
	}

	summaries := searchMetricsAll(t, s, storeCtx)
	require.Len(t, summaries, 1)
	require.Equal(t, "Cumulative", summaries[0]["aggregationTemporality"],
		"the SDK must have sent cumulative, or this test proves nothing")

	// One bucket over everything, so the merge is last-minus-first across both
	// collections.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(storeCtx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "one bucket is one merged datapoint")
	merged := dps[0].(map[string]any)

	// Compute the expected delta independently from the recorded values.
	wantCounts := make([]uint64, len(bounds)+1)
	wantSum := 0.0
	for _, v := range secondCycle {
		wantSum += v
		i := len(bounds)
		for j, b := range bounds {
			if v <= b {
				i = j
				break
			}
		}
		wantCounts[i]++
	}

	assert.Equal(t, uint64(len(secondCycle)), metricWireUint64(t, merged["count"]),
		"the merge must recover exactly the second collection's observation count")
	assert.InDelta(t, wantSum, merged["sum"], 1e-9,
		"and its sum: the first collection is the baseline and subtracts away")

	gotCounts := merged["bucketCounts"].([]any)
	require.Len(t, gotCounts, len(wantCounts))
	for i, want := range wantCounts {
		assert.Equalf(t, want, metricWireUint64(t, gotCounts[i]), "bucket %d", i)
	}
}

func TestHistogramContribution(t *testing.T) {
	for _, exponential := range []bool{false, true} {
		for _, temporality := range []pmetric.AggregationTemporality{pmetric.AggregationTemporalityDelta, pmetric.AggregationTemporalityCumulative} {
			name := temporality.String()
			if exponential {
				name += " exponential"
			}
			t.Run(name, func(t *testing.T) {
				s, ctx := storetest.New(t)
				base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
				later := base.Add(time.Minute)
				a, b := map[string]string{"pod": "a"}, map[string]string{"pod": "b"}
				fixture := makeHistogramFixtureT("contribution", temporality, []histTestDP{
					{timestamp: base, attrs: a, bounds: []float64{1}, counts: []uint64{9007199254740993, 2}, count: 9007199254740995},
					{timestamp: later, attrs: a, bounds: []float64{1}, counts: []uint64{9007199254740996, 4}, count: 9007199254741000},
					{timestamp: base, attrs: b, bounds: []float64{1}, counts: []uint64{0, 1}, count: 1},
					{timestamp: later, attrs: b, bounds: []float64{1}, counts: []uint64{0, 2}, count: 2},
				})
				if exponential {
					fixture = makeExpHistogramFixtureT("contribution", temporality, []expHistTestDP{
						{timestamp: base, attrs: a, scale: 2, posOffset: -4, posCounts: []uint64{4, 0, 0, 0, 2}, negOffset: -4, negCounts: []uint64{2, 0}, zeroCount: 1, count: 9},
						{timestamp: later, attrs: a, scale: 1, posOffset: -2, posCounts: []uint64{6, 2, 4}, negOffset: -2, negCounts: []uint64{3}, zeroCount: 2, count: 17},
						{timestamp: base, attrs: b, scale: 0, posCounts: []uint64{1}, zeroThreshold: 1, count: 1},
						{timestamp: later, attrs: b, scale: 0, posCounts: []uint64{2}, zeroThreshold: 1, count: 2},
					})
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, fixture) }))
				metricID := findMetricRef(t, s, ctx, "contribution")
				var sourceID, seriesID string
				require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
					// The selected row has the largest count at this exact timestamp.
					return db.QueryRowContext(ctx, `select id::varchar, series_id::varchar from metric_datapoints where metric_id=?::uuid and timestamp=? order by count desc limit 1`, metricID, uint64(later.UnixNano())).Scan(&sourceID, &seriesID)
				}))
				for _, target := range []int64{1, 100} {
					raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
						return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, target, nil, nil, 0, 0, nil, "", sourceID)
					})
					require.NoError(t, err)
					var got struct {
						Aggregate    []map[string]any
						Contribution []map[string]any
					}
					require.NoError(t, json.Unmarshal(raw, &got))
					require.Len(t, got.Contribution, 1)
					contribution := got.Contribution[0]
					if exponential {
						require.Equal(t, float64(0), contribution["scale"])
						require.Equal(t, float64(1), contribution["zeroThreshold"])
						if temporality == pmetric.AggregationTemporalityCumulative {
							require.Equal(t, float64(8), contribution["count"])
							require.Equal(t, float64(6), contribution["zeroCount"])
							require.Equal(t, []any{float64(2)}, contribution["positiveBucketCounts"])
						} else {
							require.Equal(t, float64(17), contribution["count"])
							require.Equal(t, float64(13), contribution["zeroCount"])
							require.Equal(t, []any{float64(4)}, contribution["positiveBucketCounts"])
						}
					} else if temporality == pmetric.AggregationTemporalityCumulative {
						require.Equal(t, float64(5), contribution["count"], "difference must happen before float projection")
						require.Equal(t, []any{float64(3), float64(2)}, contribution["bucketCounts"])
					} else {
						require.Equal(t, float64(9007199254741000), contribution["count"])
					}
					// Asking for a source contribution must not replace the existing aggregate.
					ordinary, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
						return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, target, nil, nil, 0, 0, nil, "")
					})
					require.NoError(t, err)
					var original struct{ Aggregate []map[string]any }
					require.NoError(t, json.Unmarshal(ordinary, &original))
					require.Equal(t, original.Aggregate, got.Aggregate)
				}
				// An unchecked source contributes nothing to the selected pool.
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, 1, []string{}, nil, 0, 0, nil, "", sourceID)
				})
				require.NoError(t, err)
				var empty struct{ Contribution []any }
				require.NoError(t, json.Unmarshal(raw, &empty))
				require.Empty(t, empty.Contribution)
				if temporality == pmetric.AggregationTemporalityCumulative {
					// The selected row becomes the baseline when its predecessor is outside the window.
					raw, err = storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
						return metrics.GetMetricAggregateView(ctx, db, metricID, store.BoundedTimeRange(uint64(later.UnixNano()), uint64(later.UnixNano())), 1, []string{seriesID}, nil, 0, 0, nil, "", sourceID)
					})
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(raw, &empty))
					require.Empty(t, empty.Contribution)
				}
			})
		}
	}
}

func TestHistogramContributionCumulativeReset(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	reset, later := base.Add(time.Minute), base.Add(2*time.Minute)
	a, b := map[string]string{"pod": "a"}, map[string]string{"pod": "b"}
	for _, exponential := range []bool{false, true} {
		name := "explicit"
		if exponential {
			name = "exponential"
		}
		t.Run(name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			// Pod a restarts at 10 observations, then adds three. Pod b adds two.
			// At the restart, sum and some buckets rise despite the falling count.
			// The reset must retain the whole current vector, not clamp each delta.
			fixture := makeHistogramFixtureT("reset.contribution", pmetric.AggregationTemporalityCumulative, []histTestDP{
				{timestamp: base, attrs: a, bounds: []float64{-1, 1}, counts: []uint64{7, 1, 9}, count: 17, sum: 1.5},
				{timestamp: reset, attrs: a, bounds: []float64{-1, 1}, counts: []uint64{3, 2, 5}, count: 10, sum: 4.5},
				{timestamp: later, attrs: a, bounds: []float64{-1, 1}, counts: []uint64{4, 3, 6}, count: 13, sum: 3},
				{timestamp: base, attrs: b, bounds: []float64{-1, 1}, counts: []uint64{0, 2, 0}, count: 2},
				{timestamp: later, attrs: b, bounds: []float64{-1, 1}, counts: []uint64{0, 4, 0}, count: 4},
			})
			if exponential {
				fixture = makeExpHistogramFixtureT("reset.contribution", pmetric.AggregationTemporalityCumulative, []expHistTestDP{
					{timestamp: base, attrs: a, posCounts: []uint64{8, 1}, negCounts: []uint64{6, 1}, zeroCount: 1, count: 17, sum: 1.5},
					{timestamp: reset, attrs: a, posCounts: []uint64{2, 3}, negCounts: []uint64{1, 2}, zeroCount: 2, count: 10, sum: 4.5},
					{timestamp: later, attrs: a, posCounts: []uint64{3, 3}, negCounts: []uint64{1, 3}, zeroCount: 3, count: 13, sum: 3},
					{timestamp: base, attrs: b, zeroCount: 2, count: 2},
					{timestamp: later, attrs: b, zeroCount: 4, count: 4},
				})
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, fixture) }))
			metricID := findMetricRef(t, s, ctx, "reset.contribution")
			var sourceID string
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				return db.QueryRowContext(ctx, `select id::varchar from metric_datapoints where metric_id=?::uuid and timestamp=?`, metricID, uint64(reset.UnixNano())).Scan(&sourceID)
			}))
			for _, target := range []int64{1, 100} {
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, target, nil, nil, 0, 0, nil, "", sourceID)
				})
				require.NoError(t, err)
				var got struct {
					Aggregate    []map[string]any
					Contribution []map[string]any
				}
				require.NoError(t, json.Unmarshal(raw, &got))
				require.Len(t, got.Contribution, 1)
				contribution := got.Contribution[0]
				require.Equal(t, float64(10), contribution["count"])
				require.Equal(t, float64(4.5), contribution["sum"], "a reset uses the current sum even when it increased")
				if exponential {
					require.Equal(t, float64(0), contribution["scale"])
					require.Equal(t, float64(2), contribution["zeroCount"], "a reset uses the current zero count even when it increased")
					require.Equal(t, float64(0), contribution["positiveBucketOffset"])
					require.Equal(t, float64(0), contribution["negativeBucketOffset"])
					require.Equal(t, []any{float64(2), float64(3)}, contribution["positiveBucketCounts"])
					require.Equal(t, []any{float64(1), float64(2)}, contribution["negativeBucketCounts"])
				} else {
					require.Equal(t, []any{float64(-1), float64(1)}, contribution["explicitBounds"])
					require.Equal(t, []any{float64(3), float64(2), float64(5)}, contribution["bucketCounts"])
				}

				ordinary, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, target, nil, nil, 0, 0, nil, "")
				})
				require.NoError(t, err)
				var original struct{ Aggregate []map[string]any }
				require.NoError(t, json.Unmarshal(ordinary, &original))
				require.Equal(t, original.Aggregate, got.Aggregate, "selecting the reset must leave the aggregate unchanged")
				var count, sum float64
				for _, bucket := range got.Aggregate {
					count += bucket["count"].(float64)
					sum += bucket["sum"].(float64)
				}
				require.Equal(t, float64(15), count, "aggregate includes the reset, subsequent activity, and the other series")
				require.Equal(t, float64(3), sum, "aggregate includes the post-reset decrease in sum")
			}
		})
	}
}
