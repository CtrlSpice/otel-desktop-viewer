package metrics_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestMetricSeriesOrderClampsBeforeSorting(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("order")
	points := m.SetEmptyGauge().DataPoints()
	for i, name := range []string{"item10", "item2", "item1", "item0"} {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(100 + i*100))
		p.SetIntValue(1)
		p.Attributes().PutStr("key", name)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "order")
	refs := map[string]string{}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select id::varchar, attributes.key::varchar from metric_series where metric_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref, name string
			require.NoError(t, rows.Scan(&ref, &name))
			refs[name] = ref
		}
		return rows.Err()
	}))
	for _, descending := range []bool{false, true} {
		result, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, id, store.BoundedTimeRange(100, 200), []string{refs["item10"], refs["item2"], refs["item1"], refs["item0"]}, "key", descending)
		})
		require.NoError(t, err)
		var order []string
		require.NoError(t, json.Unmarshal(result, &order))
		expected := []string{refs["item2"], refs["item10"]}
		if descending {
			expected[0], expected[1] = expected[1], expected[0]
		}
		require.Equal(t, expected, order)
	}
	start, end := uint64(200), uint64(100)
	for _, test := range []struct {
		name           string
		window         store.TimeRange
		refs, expected []string
	}{
		{"open end", store.TimeRange{Start: &start}, []string{refs["item10"], refs["item2"]}, []string{refs["item2"]}},
		{"open start", store.TimeRange{End: &end}, []string{refs["item10"], refs["item2"]}, []string{refs["item10"]}},
		{"cohort only", store.TimeRange{}, []string{refs["item2"], refs["item2"]}, []string{refs["item2"]}},
		{"reversed window", store.TimeRange{Start: &start, End: &end}, []string{refs["item10"], refs["item2"]}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeriesOrder(ctx, db, id, test.window, test.refs, "key", false)
			})
			require.NoError(t, err)
			var order []string
			require.NoError(t, json.Unmarshal(raw, &order))
			require.Equal(t, test.expected, order)
		})
	}
}

func TestMetricSeriesOrderExactValuesAndNumbersFirst(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("order.typed")
	points := m.SetEmptyGauge().DataPoints()
	const key = "'雪.[0]"
	fixtures := []struct {
		name  string
		value any
	}{
		{"negative-infinity", math.Inf(-1)}, {"min-int", int64(math.MinInt64)},
		{"negative-ten", int64(-10)}, {"negative-two", float64(-2)},
		{"minus-zero", math.Copysign(0, -1)}, {"double-zero", float64(0)}, {"int-zero", int64(0)},
		{"double-one", float64(1)}, {"int-one", int64(1)},
		{"rounded", float64(9007199254740992)}, {"exact", int64(9007199254740993)},
		{"max-int", int64(math.MaxInt64)}, {"above-max-int", float64(math.MaxInt64)},
		{"positive-infinity", math.Inf(1)}, {"nan", math.NaN()},
		{"numeric-string", "-5"}, {"empty-string", ""}, {"empty", nil},
		{"false", false}, {"true", true}, {"string-false", "false"}, {"string-nan", "NaN"},
		{"item2", "item2"}, {"item10", "item10"}, {"unicode2", "雪2"}, {"unicode10", "雪10"},
		{"lower", "e2"}, {"upper", "E2"}, {"accent", "é2"},
		{"nested2", map[string]any{"prefix": strings.Repeat("same", 1000), "tail": int64(2)}},
		{"nested10", map[string]any{"prefix": strings.Repeat("same", 1000), "tail": int64(10)}},
		{"tie-a", "same"}, {"tie-b", "same"},
		{"string-absent", "[absent]"},
	}
	for _, fixture := range fixtures {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
		p.SetIntValue(1)
		p.Attributes().PutStr("fixture", fixture.name)
		require.NoError(t, p.Attributes().PutEmpty(key).FromRaw(fixture.value))
	}
	missing := points.AppendEmpty()
	missing.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
	missing.SetIntValue(1)
	missing.Attributes().PutStr("fixture", "missing")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	var id string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `select id::varchar from metrics where name='order.typed'`).Scan(&id)
	}))
	byName := map[string]string{}
	byID := map[string]string{}
	var refs []string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select id::varchar, attributes.fixture::varchar from metric_series where metric_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref, name string
			require.NoError(t, rows.Scan(&ref, &name))
			byName[name], byID[ref] = ref, name
			refs = append(refs, ref)
		}
		return rows.Err()
	}))
	numerics := []string{"negative-infinity", "min-int", "negative-ten", "negative-two", "minus-zero", "double-zero", "int-zero", "double-one", "int-one", "rounded", "exact", "max-int", "above-max-int", "positive-infinity", "nan"}
	for _, descending := range []bool{false, true} {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, id, store.BoundedTimeRange(uint64(math.MaxUint64), uint64(math.MaxUint64)), refs, key, descending)
		})
		require.NoError(t, err)
		var ids []string
		require.NoError(t, json.Unmarshal(raw, &ids))
		require.ElementsMatch(t, refs, ids)
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			result, err := metrics.SeriesOrderArrow(ctx, db, id, store.BoundedTimeRange(uint64(math.MaxUint64), uint64(math.MaxUint64)), refs, key, descending)
			if err != nil {
				return err
			}
			defer result.Close()
			require.Len(t, result.Schema().Fields(), 1)
			var wire bytes.Buffer
			require.NoError(t, result.Write(ctx, &wire))
			reader, err := ipc.NewReader(&wire)
			if err != nil {
				return err
			}
			defer reader.Release()
			var arrowIDs []string
			for reader.Next() {
				column := reader.RecordBatch().Column(0).(*array.String)
				for i := 0; i < column.Len(); i++ {
					arrowIDs = append(arrowIDs, column.Value(i))
				}
			}
			require.NoError(t, reader.Err())
			require.Equal(t, ids, arrowIDs)
			return nil
		}))
		names := make([]string, len(ids))
		for i, id := range ids {
			names[i] = byID[id]
		}
		expectedNumbers := slices.Clone(numerics)
		if descending {
			slices.Reverse(expectedNumbers)
		}
		require.Equal(t, expectedNumbers, names[:len(numerics)])
		for _, pair := range [][2]string{{"item2", "item10"}, {"unicode2", "unicode10"}, {"nested2", "nested10"}, {"lower", "upper"}, {"upper", "accent"}, {"false", "string-false"}, {"missing", "string-absent"}} {
			left, right := slices.Index(names, pair[0]), slices.Index(names, pair[1])
			if descending {
				left, right = right, left
			}
			require.Less(t, left, right, "%v in %v", pair, names)
		}
		ties := []string{byName["tie-a"], byName["tie-b"]}
		slices.Sort(ties)
		require.Less(t, slices.Index(ids, ties[0]), slices.Index(ids, ties[1]), "identity ties stay ascending")
	}
}
