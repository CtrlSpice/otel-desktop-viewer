package nested_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func metricFixture(marker string, times ...uint64) pmetric.Metrics {
	m := pmetric.NewMetrics()
	r := m.ResourceMetrics().AppendEmpty()
	r.Resource().Attributes().PutStr("service.name", "shared-service")
	s := r.ScopeMetrics().AppendEmpty()
	s.Scope().SetName("shared-scope")
	x := s.Metrics().AppendEmpty()
	x.SetName("received-records")
	x.Metadata().PutStr("revision", marker)
	x.SetDescription(marker)
	g := x.SetEmptyGauge()
	for _, ts := range times {
		p := g.DataPoints().AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(ts))
		p.SetIntValue(int64(ts))
		p.Attributes().PutStr("label", marker)
		e := p.Exemplars().AppendEmpty()
		e.SetTimestamp(pcommon.Timestamp(ts))
		e.SetIntValue(int64(ts))
		e.FilteredAttributes().PutStr("sample", marker)
	}
	return m
}
func condition(owner, root, key, value string) search.QueryNode {
	return search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Owner: owner, Path: []any{root, key}, Type: "string"}, FieldOperator: "=", Value: value}}
}
func matches(t *testing.T, db *sql.DB, children ...search.QueryNode) int {
	t.Helper()
	q := &search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: children}}
	query, args, err := metrics.MatchingRowsSQL(q, timerange.TimeRange{})
	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), "select count(*) from ("+query+")", args...).Scan(&n))
	return n
}
func scalar(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), q).Scan(&n))
	return n
}

func TestReceivedMetricCatalogueOwnershipAndLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested.duckdb")
	s, err := store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	ingest := func(marker string, times ...uint64) {
		require.NoError(t, s.WithConn(func(c driver.Conn) error {
			return metrics.Ingest(t.Context(), c, metricFixture(marker, times...))
		}))
	}
	ingest("old", 10, 30)
	ingest("new", 20, 40)
	verify := func(db *sql.DB) error {
		require.Equal(t, 2, scalar(t, db, `select count(*) from search_owners where owner='metric'`))
		require.Equal(t, 1, scalar(t, db, `select count(*) from search_owners where owner='resource'`))
		require.Equal(t, 1, scalar(t, db, `select count(*) from search_owners where owner='scope'`))
		require.Equal(t, 4, scalar(t, db, `select count(*) from search_owners p join search_owners r on r.ref=p.parent_ref join metric_records m on m.record_id::varchar=r.record_key where p.owner='datapoint'`))
		for _, marker := range []string{"old", "new"} {
			require.Equal(t, 1, matches(t, db, condition("metric", "metadata", "revision", marker), condition("datapoint", "attributes", "label", marker)))
			require.Equal(t, 1, matches(t, db, condition("metric", "metadata", "revision", marker), condition("exemplar", "attributes", "sample", marker)))
		}
		require.Zero(t, matches(t, db, condition("metric", "metadata", "revision", "old"), condition("datapoint", "attributes", "label", "new")))
		require.Zero(t, matches(t, db, condition("datapoint", "attributes", "label", "old"), condition("exemplar", "attributes", "sample", "new")))
		raw, err := attributes.SearchScoped(t.Context(), db, attributes.SearchOptions{Signal: "metrics", Owner: "metric", Path: []any{"metadata", "revision"}})
		require.NoError(t, err)
		var found []struct {
			Count   int               `json:"matchCount"`
			Samples []json.RawMessage `json:"sampleValues"`
		}
		require.NoError(t, json.Unmarshal(raw, &found))
		require.Len(t, found, 1)
		require.Equal(t, 2, found[0].Count)
		require.Len(t, found[0].Samples, 2)
		var metricID string
		require.NoError(t, db.QueryRow(`select id::varchar from metrics`).Scan(&metricID))
		exported, err := metrics.GetMetricOTLP(t.Context(), db, metricID)
		require.NoError(t, err)
		decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(exported)
		require.NoError(t, err)
		require.Equal(t, 2, decoded.MetricCount())
		for _, resource := range decoded.ResourceMetrics().All() {
			for _, scope := range resource.ScopeMetrics().All() {
				for _, metric := range scope.Metrics().All() {
					revision, ok := metric.Metadata().Get("revision")
					require.True(t, ok)
					require.Equal(t, metric.Description(), revision.Str())
					require.Equal(t, 2, metric.Gauge().DataPoints().Len())
					for _, point := range metric.Gauge().DataPoints().All() {
						label, ok := point.Attributes().Get("label")
						require.True(t, ok)
						require.Equal(t, metric.Description(), label.Str(), "each point keeps its received descriptor")
					}
				}
			}
		}
		return nil
	}
	require.NoError(t, s.WithDBRead(verify))
	require.NoError(t, s.Close())
	s, err = store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.WithDBRead(verify))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		require.NoError(t, nested.PrunePoints(t.Context(), db, 15))
		require.Equal(t, 3, scalar(t, db, `select count(*) from metric_datapoints`))
		require.Equal(t, 3, scalar(t, db, `select count(*) from search_owners where owner='exemplar'`))
		require.Equal(t, 30, scalar(t, db, `select point_time_min from metric_records where description='old'`))
		require.NoError(t, nested.Mutate(t.Context(), db, `delete from metric_records where description='old'`))
		require.Zero(t, matches(t, db, condition("metric", "metadata", "revision", "old")))
		require.Equal(t, 1, scalar(t, db, `select count(*) from search_owners where owner='resource'`))
		require.NoError(t, metrics.Clear(t.Context(), db))
		for _, table := range []string{"metric_records", "search_owners", "search_paths", "search_values", "search_postings"} {
			require.Zero(t, scalar(t, db, "select count(*) from "+table), table)
		}
		return nil
	}))
}
