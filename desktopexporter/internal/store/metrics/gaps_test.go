package metrics_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
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
	read := func(buckets int64) []series {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, window, buckets, nil, nil, 0, 0, 1, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var view struct {
			Timeseries []series `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		return view.Timeseries
	}
	stamp := func(second int64) string { return strconv.FormatInt(base+second*int64(time.Second), 10) }
	rows := read(6) // 10-second M4 buckets, but a single coarse sparkline bucket.
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
	require.Equal(t, 1, sparkBreaks, "coarser sparkline reduction must retain the break")
	for _, buckets := range []int64{0, 1} {
		for _, row := range read(buckets) {
			for _, p := range append(row.Datapoints, row.Sparkline...) {
				require.Nil(t, p.BreakBefore, "no empty M4 bucket means no inferred break")
			}
		}
	}
	// The native Arrow path carries true/null, never false. Both transports
	// retain the same break on main-chart and sparkline points.
	_, err := readStore(s, func(db *sql.DB) (int, error) {
		result, err := metrics.ViewArrow(ctx, db, id, window, 6, nil, nil, 0, 0, 1, nil, "", nil, 0, false, "")
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
