package metrics_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

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
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs()) }))
				metricID := findMetricRef(t, s, ctx, "contribution")
				var sourceID, seriesID string
				require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
					// The selected row has the largest count at this exact timestamp.
					return db.QueryRowContext(ctx, `select id::varchar, series_id::varchar from metric_datapoints where metric_id=?::uuid and timestamp=? order by count desc limit 1`, metricID, uint64(later.UnixNano())).Scan(&sourceID, &seriesID)
				}))
				for _, target := range []int64{1, 100} {
					raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
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
					ordinary, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
						return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, target, nil, nil, 0, 0, nil, "")
					})
					require.NoError(t, err)
					var original struct{ Aggregate []map[string]any }
					require.NoError(t, json.Unmarshal(ordinary, &original))
					require.Equal(t, original.Aggregate, got.Aggregate)
				}
				// An unchecked source contributes nothing to the selected pool.
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricAggregateView(ctx, db, metricID, store.TimeRange{}, 1, []string{}, nil, 0, 0, nil, "", sourceID)
				})
				require.NoError(t, err)
				var empty struct{ Contribution []any }
				require.NoError(t, json.Unmarshal(raw, &empty))
				require.Empty(t, empty.Contribution)
				if temporality == pmetric.AggregationTemporalityCumulative {
					// The selected row becomes the baseline when its predecessor is outside the window.
					raw, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
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
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs()) }))
			metricID := findMetricRef(t, s, ctx, "reset.contribution")
			var sourceID string
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				return db.QueryRowContext(ctx, `select id::varchar from metric_datapoints where metric_id=?::uuid and timestamp=?`, metricID, uint64(reset.UnixNano())).Scan(&sourceID)
			}))
			for _, target := range []int64{1, 100} {
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
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

				ordinary, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
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
