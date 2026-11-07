package nested_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Only attributes contain this integer. Its physical BIGINT segment proves
// shredding below the owning LIST/STRUCT, rather than merely typed point fields.
func TestNestedAttributesShredAndReopen(t *testing.T) {
	const exact = int64(9007199254740993)
	path := filepath.Join(t.TempDir(), "shredded.duckdb")
	s, err := store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	traces := ptrace.NewTraces()
	ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	metric := metricFixture("shredded", 10)
	points := metric.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints()
	points.At(0).Attributes().PutInt("exact", exact)
	points.At(0).Exemplars().At(0).FilteredAttributes().PutInt("exact", exact)
	for i := range 256 {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{1})
		var id pcommon.SpanID
		binary.BigEndian.PutUint64(id[:], uint64(i+1))
		span.SetSpanID(id)
		span.Events().AppendEmpty().Attributes().PutInt("exact", exact)
		span.Links().AppendEmpty().Attributes().PutInt("exact", exact)
		if i > 0 {
			points.At(0).CopyTo(points.AppendEmpty())
		}
	}
	require.NoError(t, s.WithConn(func(c driver.Conn) error {
		if err := spans.Ingest(t.Context(), c, traces); err != nil {
			return err
		}
		return metrics.Ingest(t.Context(), c, metric)
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec("checkpoint")
		return err
	}))
	require.NoError(t, s.Close())
	s, err = store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"events", "links", "metric_datapoints", "exemplars"} {
			var count int
			require.NoError(t, db.QueryRow("select count(*) from "+table+" where attributes.exact::bigint=?", exact).Scan(&count))
			require.Equal(t, 256, count, table)
		}
		for _, location := range [][2]string{{"spans", "events"}, {"spans", "links"}, {"metric_records", "datapoints"}} {
			var count int
			require.NoError(t, db.QueryRow(`select count(*) from pragma_storage_info(?)
				where column_name=? and segment_type='BIGINT' and stats like '%9007199254740993%'`, location[0], location[1]).Scan(&count))
			require.Positive(t, count, "physical nested attribute segment: %v", location)
		}
		return nil
	}))
}
