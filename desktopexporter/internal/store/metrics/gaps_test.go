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
	rows := read(6, 6)
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
			// The preview keeps extrema, not extra exact gap boundaries.
			require.Equal(t, stamp(41), p.Timestamp)
			require.Greater(t, i, 0)
			require.Equal(t, stamp(2), row.Sparkline[i-1].Timestamp)
			require.Equal(t, float64(100), row.Sparkline[i-1].Value)
			require.Equal(t, float64(-5), p.Value)
		}
	}
	require.Equal(t, 1, breaks, "other series cannot fill a series' empty buckets")
	require.Equal(t, 1, sparkBreaks, "sparkline extrema inherit the main chart's break")
	for _, buckets := range []int64{0, 1} {
		for _, row := range read(buckets, 1) {
			for _, p := range append(row.Datapoints, row.Sparkline...) {
				require.Nil(t, p.BreakBefore, "no empty M4 bucket means no inferred break")
			}
		}
	}
	for _, row := range read(6, 1) {
		require.LessOrEqual(t, len(row.Sparkline), 2, "gaps must not add preview points beyond its budget")
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
				// Preview breaks come from the main chart, before numeric filtering.
				return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(base, base+int64(time.Minute)-1),
					6, nil, nil, 0, 0, 6, nil, "", nil, 0)
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
				require.Equal(t, strconv.FormatInt(base+42*int64(time.Second), 10), p.Timestamp)
				require.Greater(t, i, 0)
				require.Equal(t, strconv.FormatInt(base+2*int64(time.Second), 10), view.Timeseries[0].Sparkline[i-1].Timestamp)
			}
			if tc.fillBuckets {
				require.Zero(t, breaks)
			} else {
				require.Equal(t, 1, breaks)
			}
		})
	}
}

func TestGaugeGapsUseBucketOccupancyDuringRepeatedHours(t *testing.T) {
	for _, tc := range []struct {
		name          string
		clocks        []string
		occupiedClock string
		buckets       int64
		boundaries    map[string]string // following sample -> preceding sample, in UTC
	}{
		{
			name:          "half-hour buckets",
			clocks:        []string{"04:15", "04:45", "05:15", "06:15", "07:15", "07:45"},
			occupiedClock: "05:45", buckets: 8,
			boundaries: map[string]string{"07:15": "06:15"},
		},
		{
			name:          "both passes of the repeated hour",
			clocks:        []string{"04:45", "05:00", "05:30", "05:45", "06:00", "06:30", "06:45", "07:00", "07:15", "07:30", "07:45"},
			occupiedClock: "05:15", buckets: 16,
			boundaries: map[string]string{"05:30": "05:00", "06:30": "06:00"},
		},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(tc.name+"/missing="+strconv.FormatBool(missing), func(t *testing.T) {
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				m.SetName("calendar-occupancy")
				points := m.SetEmptyGauge().DataPoints()
				stamp := func(clock string) int64 {
					timestamp, err := time.Parse(time.RFC3339, "2026-11-01T"+clock+":00Z")
					require.NoError(t, err)
					return timestamp.UnixNano()
				}
				clocks := append([]string{}, tc.clocks...)
				if !missing {
					clocks = append(clocks, tc.occupiedClock)
				}
				for _, clock := range clocks {
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.Timestamp(stamp(clock)))
					p.SetIntValue(0)
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
				id := findMetricRef(t, s, ctx, "calendar-occupancy")
				start := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(start.UnixNano(), start.Add(4*time.Hour).UnixNano()-1),
						tc.buckets, nil, nil, 0, 0, tc.buckets, nil, "America/New_York", nil, 0)
				})
				require.NoError(t, err)
				type point struct {
					Timestamp   string `json:"timestamp"`
					BreakBefore *bool  `json:"breakBefore"`
				}
				var view struct {
					Timeseries []struct {
						Datapoints []point `json:"datapoints"`
						Sparkline  []point `json:"sparkline"`
					} `json:"timeseries"`
				}
				require.NoError(t, json.Unmarshal(raw, &view))
				require.Len(t, view.Timeseries, 1)
				want := map[string]string{}
				if missing {
					for after, before := range tc.boundaries {
						want[strconv.FormatInt(stamp(after), 10)] = strconv.FormatInt(stamp(before), 10)
					}
				}
				for name, points := range map[string][]point{
					"main":      view.Timeseries[0].Datapoints,
					"sparkline": view.Timeseries[0].Sparkline,
				} {
					got := map[string]string{}
					for i, p := range points {
						if p.BreakBefore == nil {
							continue
						}
						require.True(t, *p.BreakBefore)
						previous := i - 1
						if name == "main" {
							previous = i + 1
						}
						require.GreaterOrEqual(t, previous, 0)
						require.Less(t, previous, len(points))
						got[p.Timestamp] = points[previous].Timestamp
					}
					require.Equal(t, want, got, name)
				}
			})
		}
	}
}

func TestGaugeGapsFollowCalendarBuckets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		times   []string
		buckets int64
		breaks  int
		zone    string
	}{
		{"spring skipped hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T08:30:00Z"}, 2, 0, "America/New_York"},
		{"spring real empty hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T09:30:00Z"}, 3, 1, "America/New_York"},
		{"fall 25-hour day", []string{"2026-10-31T16:00:00Z", "2026-11-01T17:00:00Z", "2026-11-02T17:00:00Z"}, 3, 0, "America/New_York"},
		{"Apia skipped day", []string{"2011-12-29T22:00:00Z", "2011-12-30T22:00:00Z", "2011-12-31T22:00:00Z"}, 3, 0, "Pacific/Apia"},
		{"Apia real empty day", []string{"2011-12-29T22:00:00Z", "2011-12-30T22:00:00Z", "2012-01-02T22:00:00Z"}, 4, 1, "Pacific/Apia"},
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
				return metrics.GetMetricView(ctx, db, id, store.TimeRange{}, tc.buckets, nil, nil, 0, 0, 0, nil, tc.zone, nil, 0)
			})
			require.NoError(t, err)
			require.Equal(t, tc.breaks, bytes.Count(raw, []byte(`"breakBefore":true`)))
		})
	}
}

func TestGaugeGapsAcrossClockChanges(t *testing.T) {
	for _, tc := range []struct {
		name, zone, start, outage string
		windowHours               int
		buckets                   int64
		aliasOnly                 bool
	}{
		{"Chatham spring", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 4, 8, false},
		{"Chatham spring UI resolution", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 172, 500, false},
		{"Chatham skipped label only", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 4, 8, true},
		{"Chatham fall", "Pacific/Chatham", "2026-04-04T12:00:00Z", "2026-04-04T15:15:00Z", 4, 8, false},
		{"Lord Howe half-hour spring", "Australia/Lord_Howe", "2026-10-03T14:00:00Z", "2026-10-03T17:00:00Z", 4, 8, false},
		{"Lord Howe half-hour fall", "Australia/Lord_Howe", "2026-04-04T14:00:00Z", "2026-04-04T17:00:00Z", 4, 8, false},
		{"New York spring", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-08T08:00:00Z", 4, 8, false},
		{"Kathmandu quarter-hour change", "Asia/Kathmandu", "1985-12-31T17:00:00Z", "1985-12-31T20:15:00Z", 4, 8, false},
		{"Kathmandu quarter-hour buckets", "Asia/Kathmandu", "1985-12-31T17:00:00Z", "1985-12-31T20:15:00Z", 4, 16, false},
		{"Monrovia seconds offset change", "Africa/Monrovia", "1972-01-06T23:00:00Z", "1972-01-07T02:00:00Z", 4, 8, false},
		{"Apia skipped date", "Pacific/Apia", "2011-12-30T08:00:00Z", "2011-12-30T11:00:00Z", 4, 8, false},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(tc.name+"/missing="+strconv.FormatBool(missing), func(t *testing.T) {
				parse := func(text string) time.Time {
					timestamp, err := time.Parse(time.RFC3339, text)
					require.NoError(t, err)
					return timestamp
				}
				start, outage := parse(tc.start), parse(tc.outage)
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				m.SetName("clock-change")
				points := m.SetEmptyGauge().DataPoints()
				var before, after time.Time
				for timestamp := start; timestamp.Before(start.Add(4 * time.Hour)); timestamp = timestamp.Add(5 * time.Minute) {
					// Keep the skipped 03:30 label's samples, but omit all samples
					// labelled 04:30 that resolve to the same UTC bucket. Choosing
					// max(source label) must not substitute for resolving the bucket.
					if tc.aliasOnly && !timestamp.Before(start.Add(165*time.Minute)) && timestamp.Before(start.Add(195*time.Minute)) {
						continue
					}
					if missing && !timestamp.Before(outage) && timestamp.Before(outage.Add(30*time.Minute)) {
						continue
					}
					if timestamp.Before(outage) {
						before = timestamp
					} else if after.IsZero() {
						after = timestamp
					}
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
					p.SetIntValue(0)
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
				id := findMetricRef(t, s, ctx, "clock-change")
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					window := store.BoundedTimeRange(start.UnixNano(), start.Add(time.Duration(tc.windowHours)*time.Hour).UnixNano()-1)
					return metrics.GetMetricView(ctx, db, id, window, tc.buckets, nil, nil, 0, 0, 0, nil, tc.zone, nil, 0)
				})
				require.NoError(t, err)
				var view struct {
					Timeseries []struct {
						Datapoints []struct {
							Timestamp   string `json:"timestamp"`
							BreakBefore *bool  `json:"breakBefore"`
						} `json:"datapoints"`
					} `json:"timeseries"`
				}
				require.NoError(t, json.Unmarshal(raw, &view))
				require.Len(t, view.Timeseries, 1)
				got := map[string]string{}
				for i, p := range view.Timeseries[0].Datapoints {
					if p.BreakBefore == nil {
						continue
					}
					require.True(t, *p.BreakBefore)
					require.Less(t, i+1, len(view.Timeseries[0].Datapoints))
					got[p.Timestamp] = view.Timeseries[0].Datapoints[i+1].Timestamp
				}
				want := map[string]string{}
				if missing {
					want[strconv.FormatInt(after.UnixNano(), 10)] = strconv.FormatInt(before.UnixNano(), 10)
				}
				require.Equal(t, want, got, "breaks must retain the actual samples surrounding only the empty bucket")
			})
		}
	}
}

func TestGaugeGapsAcrossSkippedDateAtMillisecondResolution(t *testing.T) {
	s, ctx := storetest.New(t)
	transition := time.Date(2011, 12, 30, 10, 0, 0, 0, time.UTC)
	start := transition.Add(-time.Second)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("skipped-date-fine-grid")
	points := m.SetEmptyGauge().DataPoints()
	for i := 0; i < 2000; i++ {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i) * time.Millisecond)))
		p.SetIntValue(0)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "skipped-date-fine-grid")
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(start.UnixNano(), start.Add(2*time.Second).UnixNano()-1),
			500, nil, nil, 0, 0, 0, nil, "Pacific/Apia", nil, 0)
	})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"timestamp"`)
	require.NotContains(t, string(raw), `"breakBefore":true`)
}
