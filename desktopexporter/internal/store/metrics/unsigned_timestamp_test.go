package metrics_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

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
		return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select d.timestamp, d.start_time, e.timestamp
			from datapoints d join exemplars e on e.datapoint_id = d.id order by d.timestamp`)
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

	summariesRaw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summariesRaw, &summaries))
	require.Equal(t, "18446744073709551615", summaries[0]["lastSeen"])

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"timestamp":"18446744073709551615"`)
	require.Contains(t, string(raw), `"startTime":"18446744073709551615"`)
}
