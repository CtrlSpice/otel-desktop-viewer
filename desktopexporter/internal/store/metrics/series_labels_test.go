package metrics_test

import (
	"bytes"
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

func TestNestedComparisonPayloadLoadsOnlyOnDemand(t *testing.T) {
	var viewBytes []int
	for _, size := range []int{1024, 65536} {
		s, ctx := storetest.New(t)
		data := pmetric.NewMetrics()
		m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("nested-only")
		points := m.SetEmptyGauge().DataPoints()
		const key = "context\"雪"
		for i := range 4 {
			p := points.AppendEmpty()
			p.SetTimestamp(100)
			p.SetIntValue(1)
			value := p.Attributes().PutEmptyMap(key)
			value.PutStr("prefix", strings.Repeat("x", size))
			value.PutInt("tail", int64(i)+9007199254740992)
			p.Attributes().PutStr("unrelated", "do not ship this attribute")
		}
		require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
		id := findMetricRef(t, s, ctx, "nested-only")
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", []string{}, 0)
		})
		require.NoError(t, err)
		require.NotContains(t, string(raw), strings.Repeat("x", 256))
		var view struct {
			Timeseries []struct {
				SeriesRef  string `json:"seriesRef"`
				Attributes []any  `json:"attributes"`
				Previews   []struct {
					Key  string `json:"key"`
					Kind string `json:"kind"`
				} `json:"labelPreviews"`
			} `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		require.Len(t, view.Timeseries, 4)
		refs := make([]string, 0, 4)
		for _, row := range view.Timeseries {
			require.Empty(t, row.Attributes)
			require.Len(t, row.Previews, 1)
			require.Equal(t, key, row.Previews[0].Key)
			require.Equal(t, "map", row.Previews[0].Kind)
			refs = append(refs, row.SeriesRef)
		}
		_, err = readStore(s, func(db *sql.DB) (int, error) {
			result, err := metrics.ViewArrow(ctx, db, id, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", []string{}, 0, false, "")
			if err != nil {
				return 0, err
			}
			defer result.Close()
			var wire bytes.Buffer
			if err := result.Write(ctx, &wire); err != nil {
				return 0, err
			}
			viewBytes = append(viewBytes, wire.Len())
			return wire.Len(), nil
		})
		require.NoError(t, err)
		column, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesColumn(ctx, db, id, refs, key)
		})
		require.NoError(t, err)
		require.Contains(t, string(column), strings.Repeat("x", size))
		require.Contains(t, string(column), "9007199254740995")
		require.NotContains(t, string(column), "unrelated")
		t.Logf("nested bytes/value=%d initial JSON=%d Arrow=%d exact-column JSON=%d", size, len(raw), viewBytes[len(viewBytes)-1], len(column))
	}
	// Increasing each comparison value by 63KiB must not increase the initial
	// response proportionally. IDs/previews have the same bounded shape.
	require.LessOrEqual(t, viewBytes[1], viewBytes[0]+256)
}

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
			require.Empty(t, series.Attributes, "view carries previews, not exact comparison payloads")
			require.False(t, series.Complete)
			require.Len(t, series.Previews, 1)
			require.Equal(t, "varying", series.Previews[0].Key)
			require.Less(t, len(series.Previews[0].Text), 512)
			require.False(t, previews[series.Previews[0].Text], "bounded preview collisions must be disambiguated")
			previews[series.Previews[0].Text] = true
			for _, attr := range bySeries[series.SeriesRef] {
				require.Equal(t, "varying", attr["key"])
			}
			encoded, err := json.Marshal(bySeries[series.SeriesRef])
			require.NoError(t, err)
			require.False(t, signatures[string(encoded)], "typed label candidates collapsed")
			signatures[string(encoded)] = true
			received, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, metricID, series.SeriesRef, store.BoundedTimeRange(0, maxNano))
			})
			require.NoError(t, err)
			require.Contains(t, string(received), `"common"`)
			require.Contains(t, string(received), `"retained"`)
			var exact struct {
				Attributes []map[string]any `json:"attributes"`
			}
			require.NoError(t, json.Unmarshal(received, &exact))
			selected := []map[string]any{}
			for _, attr := range exact.Attributes {
				if attr["key"] == "varying" {
					selected = append(selected, attr)
				}
			}
			require.Equal(t, selected, bySeries[series.SeriesRef])
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
