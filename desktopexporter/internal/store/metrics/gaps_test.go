package metrics_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestGaugeGapsPreserveRealBoundaries(t *testing.T) {
	s, ctx := storetest.New(t)
	base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano()
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("gaps")
	points := m.SetEmptyGauge().DataPoints()
	add := func(series string, second int64, value int64) {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(base + second*int64(time.Second)))
		p.SetIntValue(value)
		p.Attributes().PutStr("series", series)
	}
	for _, p := range [][2]int64{{0, 0}, {1, -10}, {2, 100}, {3, 42}, {40, 43}, {40, 44}, {41, -5}, {42, 99}, {49, 0}} {
		add("interrupted", p[0], p[1])
	}
	for second := int64(1); second < 60; second += 10 {
		add("continuous", second, 0)
	}
	add("singleton", 50, 1)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "gaps")
	window := store.BoundedTimeRange(base, base+int64(time.Minute)-1)
	type point struct {
		ID          string  `json:"id"`
		Timestamp   string  `json:"timestamp"`
		IntValue    string  `json:"intValue"`
		Value       float64 `json:"value"`
		BreakBefore *bool   `json:"breakBefore"`
	}
	type series struct {
		Datapoints []point `json:"datapoints"`
		Sparkline  []point `json:"sparkline"`
	}
	read := func(buckets, sparkBuckets int64) []series {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, window, buckets, nil, nil, 0, 0, sparkBuckets, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var view struct {
			Timeseries []series `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		return view.Timeseries
	}
	stamp := func(second int64) string { return strconv.FormatInt(base+second*int64(time.Second), 10) }
	rows := read(6, 6) // Matching 10-second grids detect the same outage independently.
	require.Len(t, rows, 3)
	breaks, sparkBreaks := 0, 0
	for _, row := range rows {
		for i, p := range row.Datapoints {
			if p.BreakBefore == nil {
				continue
			}
			breaks++
			require.True(t, *p.BreakBefore)
			require.Equal(t, stamp(40), p.Timestamp)
			// Newest-first transport: this is the first point after the gap in
			// reverse traversal, including when timestamps are duplicated.
			require.Less(t, i+1, len(row.Datapoints))
			require.Equal(t, stamp(3), row.Datapoints[i+1].Timestamp)
			require.Equal(t, "42", row.Datapoints[i+1].IntValue)
		}
		for i, p := range row.Sparkline {
			if p.BreakBefore == nil {
				continue
			}
			sparkBreaks++
			require.True(t, *p.BreakBefore)
			require.Equal(t, stamp(40), p.Timestamp)
			require.Greater(t, i, 0)
			require.Equal(t, stamp(3), row.Sparkline[i-1].Timestamp)
			require.Equal(t, float64(42), row.Sparkline[i-1].Value)
			require.Contains(t, []float64{43, 44}, p.Value)
		}
	}
	require.Equal(t, 1, breaks, "other series cannot fill a series' empty buckets")
	require.Equal(t, 1, sparkBreaks, "sparkline reduction must retain its own detected break")
	for _, buckets := range []int64{0, 1} {
		for _, row := range read(buckets, 1) {
			for _, p := range append(row.Datapoints, row.Sparkline...) {
				require.Nil(t, p.BreakBefore, "no empty M4 bucket means no inferred break")
			}
		}
	}
	for _, row := range read(6, 1) {
		for _, p := range row.Sparkline {
			require.Nil(t, p.BreakBefore, "a coarse sparkline need not show the main chart's gap")
		}
	}
	for _, row := range read(6, 0) {
		require.Nil(t, row.Sparkline, "disabled sparklines remain absent")
	}
	// The native Arrow path carries true/null, never false. Both transports
	// retain the same break on main-chart and sparkline points.
	_, err := readStore(s, func(db *sql.DB) (int, error) {
		result, err := metrics.ViewArrow(ctx, db, id, window, 6, nil, nil, 0, 0, 6, nil, "", nil, 0, false, "")
		if err != nil {
			return 0, err
		}
		defer result.Close()
		var wire bytes.Buffer
		if err := result.Write(ctx, &wire); err != nil {
			return 0, err
		}
		reader, err := ipc.NewReader(&wire)
		if err != nil {
			return 0, err
		}
		defer reader.Release()
		require.True(t, reader.Next())
		record := reader.RecordBatch()
		rows := record.Column(record.Schema().FieldIndices("timeseries")[0]).(*array.List).ListValues().(*array.Struct)
		field := func(rows *array.Struct, name string) arrow.Array {
			index, ok := rows.DataType().(*arrow.StructType).FieldIdx(name)
			require.True(t, ok, name)
			return rows.Field(index)
		}
		for _, name := range []string{"datapoints", "sparkline"} {
			points := field(rows, name).(*array.List).ListValues().(*array.Struct)
			flags := field(points, "breakBefore").(*array.Boolean)
			count := 0
			for i := 0; i < flags.Len(); i++ {
				if flags.IsNull(i) {
					continue
				}
				require.True(t, flags.Value(i))
				count++
			}
			require.Equal(t, 1, count, name)
		}
		return 0, reader.Err()
	})
	require.NoError(t, err)
}

func TestSparklineGapsSurviveNumericFiltering(t *testing.T) {
	for _, tc := range []struct {
		name           string
		value          float64
		empty          bool
		beforeInvalid  bool
		fillBuckets    bool
		invalidSection bool
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "empty", empty: true},
		{name: "both boundaries invalid", value: math.NaN(), beforeInvalid: true},
		{name: "invalid-only section", value: math.NaN(), invalidSection: true},
		{name: "invalid readings occupy buckets", value: math.NaN(), fillBuckets: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano()
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("sparkline-filtering")
			points := m.SetEmptyGauge().DataPoints()
			for _, sample := range []struct {
				second int64
				value  float64
			}{
				{0, 0}, {1, -10}, {2, 100}, {3, 42}, {40, 44}, {41, 43}, {41, 44}, {42, 99}, {49, 0},
			} {
				p := points.AppendEmpty()
				p.SetTimestamp(pcommon.Timestamp(base + sample.second*int64(time.Second)))
				if sample.second == 40 || (sample.second == 3 && tc.beforeInvalid) {
					if !tc.empty {
						p.SetDoubleValue(tc.value)
					}
				} else {
					p.SetDoubleValue(sample.value)
				}
			}
			for _, second := range []int64{10, 20, 30} {
				if tc.fillBuckets || (tc.invalidSection && second == 20) {
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.Timestamp(base + second*int64(time.Second)))
					if second != 10 {
						p.SetDoubleValue(math.NaN())
					}
				}
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id := findMetricRef(t, s, ctx, "sparkline-filtering")
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				// Disable main-chart reduction: sparkline gaps must be independent.
				return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(base, base+int64(time.Minute)-1),
					0, nil, nil, 0, 0, 6, nil, "", nil, 0)
			})
			require.NoError(t, err)
			var view struct {
				Timeseries []struct {
					Sparkline []struct {
						Timestamp   string  `json:"timestamp"`
						Value       float64 `json:"value"`
						BreakBefore *bool   `json:"breakBefore"`
					} `json:"sparkline"`
				} `json:"timeseries"`
			}
			require.NoError(t, json.Unmarshal(raw, &view))
			require.Len(t, view.Timeseries, 1)
			breaks := 0
			for i, p := range view.Timeseries[0].Sparkline {
				if p.BreakBefore == nil {
					continue
				}
				breaks++
				require.True(t, *p.BreakBefore)
				require.Equal(t, strconv.FormatInt(base+41*int64(time.Second), 10), p.Timestamp)
				require.Greater(t, i, 0)
				before := int64(3)
				if tc.beforeInvalid {
					before = 2
				}
				require.Equal(t, strconv.FormatInt(base+before*int64(time.Second), 10), view.Timeseries[0].Sparkline[i-1].Timestamp)
			}
			if tc.fillBuckets {
				require.Zero(t, breaks)
			} else {
				require.Equal(t, 1, breaks)
			}
		})
	}
}

func TestSparklineGapsUseBucketOccupancyDuringRepeatedHours(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(strconv.FormatBool(missing), func(t *testing.T) {
			s, ctx := storetest.New(t)
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("sparkline-calendar")
			points := m.SetEmptyGauge().DataPoints()
			for _, clock := range []string{"04:15", "04:45", "05:15", "05:45", "06:15", "07:15", "07:45"} {
				if missing && clock == "05:45" {
					continue
				}
				timestamp, err := time.Parse(time.RFC3339, "2026-11-01T"+clock+":00Z")
				require.NoError(t, err)
				p := points.AppendEmpty()
				p.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
				p.SetIntValue(0)
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id := findMetricRef(t, s, ctx, "sparkline-calendar")
			start := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(start.UnixNano(), start.Add(4*time.Hour).UnixNano()-1),
					0, nil, nil, 0, 0, 8, nil, "America/New_York", nil, 0)
			})
			require.NoError(t, err)
			want := 0
			if missing {
				want = 1
			}
			require.Equal(t, want, bytes.Count(raw, []byte(`"breakBefore":true`)),
				"the repeated 01:30 bucket is occupied by 05:45Z unless that sample is omitted")
		})
	}
}

func TestGaugeGapsFollowCalendarBuckets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		times   []string
		buckets int64
		breaks  int
	}{
		{"spring skipped hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T08:30:00Z"}, 2, 0},
		{"spring real empty hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T09:30:00Z"}, 3, 1},
		{"fall 25-hour day", []string{"2026-10-31T16:00:00Z", "2026-11-01T17:00:00Z", "2026-11-02T17:00:00Z"}, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("calendar")
			points := m.SetEmptyGauge().DataPoints()
			for _, text := range tc.times {
				timestamp, err := time.Parse(time.RFC3339, text)
				require.NoError(t, err)
				p := points.AppendEmpty()
				p.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
				p.SetIntValue(0)
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id := findMetricRef(t, s, ctx, "calendar")
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, id, store.TimeRange{}, tc.buckets, nil, nil, 0, 0, 0, nil, "America/New_York", nil, 0)
			})
			require.NoError(t, err)
			require.Equal(t, tc.breaks, bytes.Count(raw, []byte(`"breakBefore":true`)))
		})
	}
}
