package metrics_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestSeriesLabelProjectionPreservesTypedDistinctionsAndReceivedContext(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "labels")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("labels.typed")
	points := m.SetEmptyGauge().DataPoints()
	fixtures := []func(pcommon.Map){
		func(a pcommon.Map) {},
		func(a pcommon.Map) { a.PutEmpty("varying") },
		func(a pcommon.Map) { a.PutStr("varying", "") },
		func(a pcommon.Map) { a.PutInt("varying", 9007199254740992) },
		func(a pcommon.Map) { a.PutInt("varying", 9007199254740993) },
		func(a pcommon.Map) { a.PutDouble("varying", 0) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Copysign(0, -1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Inf(1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Inf(-1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.NaN()) },
		func(a pcommon.Map) { a.PutStr("varying", "NaN") },
		func(a pcommon.Map) { a.PutEmptyMap("varying").PutInt("n", 1) },
		func(a pcommon.Map) { a.PutEmptyMap("varying").PutDouble("n", 1) },
		func(a pcommon.Map) { a.PutEmptySlice("varying").AppendEmpty().SetDouble(math.Copysign(0, -1)) },
		func(a pcommon.Map) {
			a.PutEmptyMap("varying").PutEmptySlice("nested").AppendEmpty().SetStr(strings.Repeat("received ", 3000))
		},
	}
	for _, fixture := range fixtures {
		p := points.AppendEmpty()
		p.SetTimestamp(100)
		p.SetIntValue(1)
		p.Attributes().PutEmptyMap("common").PutEmptySlice("nested").AppendEmpty().SetStr("retained")
		fixture(p.Attributes())
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	metricID := findMetricRef(t, s, ctx, "labels.typed")
	for _, shipping := range [][]string{nil, {}} {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", shipping, 0)
		})
		require.NoError(t, err)
		var view struct {
			Timeseries []struct {
				SeriesRef  string           `json:"seriesRef"`
				Attributes []map[string]any `json:"attributes"`
				Complete   bool             `json:"attributesComplete"`
				Previews   []struct {
					Key  string `json:"key"`
					Text string `json:"text"`
				} `json:"labelPreviews"`
			} `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		require.Len(t, view.Timeseries, len(fixtures))
		ids := make([]string, len(view.Timeseries))
		for i, series := range view.Timeseries {
			ids[i] = series.SeriesRef
		}
		column, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesColumn(ctx, db, metricID, ids, "varying")
		})
		require.NoError(t, err)
		var cells []struct {
			SeriesRef  string           `json:"seriesRef"`
			Attributes []map[string]any `json:"attributes"`
		}
		require.NoError(t, json.Unmarshal(column, &cells))
		require.Len(t, cells, len(view.Timeseries))
		bySeries := make(map[string][]map[string]any)
		for _, cell := range cells {
			bySeries[cell.SeriesRef] = cell.Attributes
		}
		signatures := make(map[string]bool)
		previews := make(map[string]bool)
		for _, series := range view.Timeseries {
			require.Equal(t, series.Attributes, bySeries[series.SeriesRef])
			require.False(t, series.Complete)
			require.Len(t, series.Previews, 1)
			require.Equal(t, "varying", series.Previews[0].Key)
			require.Less(t, len(series.Previews[0].Text), 512)
			require.False(t, previews[series.Previews[0].Text], "bounded preview collisions must be disambiguated")
			previews[series.Previews[0].Text] = true
			for _, attr := range series.Attributes {
				require.Equal(t, "varying", attr["key"])
			}
			encoded, err := json.Marshal(series.Attributes)
			require.NoError(t, err)
			require.False(t, signatures[string(encoded)], "typed label candidates collapsed")
			signatures[string(encoded)] = true
			received, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, metricID, series.SeriesRef, store.BoundedTimeRange(0, maxNano))
			})
			require.NoError(t, err)
			require.Contains(t, string(received), `"common"`)
			require.Contains(t, string(received), `"retained"`)
			contextOnly, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, metricID, series.SeriesRef, store.BoundedTimeRange(0, maxNano), false)
			})
			require.NoError(t, err)
			require.Contains(t, string(contextOnly), `"datapoints":[]`)
			require.Contains(t, string(contextOnly), `"retained"`)
		}
		for _, refs := range [][]string{{}, {"00000000-0000-0000-0000-000000000000"}} {
			empty, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeriesColumn(ctx, db, metricID, refs, "varying")
			})
			require.NoError(t, err)
			require.JSONEq(t, "[]", string(empty))
		}
		foreign, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesColumn(ctx, db, "00000000-0000-0000-0000-000000000000", ids, "varying")
		})
		require.NoError(t, err)
		require.JSONEq(t, "[]", string(foreign))
		missing, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesColumn(ctx, db, metricID, []string{ids[0], ids[0]}, "'; select 1; --")
		})
		require.NoError(t, err)
		require.JSONEq(t, `[{"seriesRef":"`+ids[0]+`","attributes":[]}]`, string(missing))
	}
}
