package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// sweepBenchmarkMetrics models one recurring instrument reported over many
// batches. Building it as repeated Metric entries creates the same persisted
// metric_ingests shape in one setup call, outside the timed sweep loop.
func sweepBenchmarkMetrics(count int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "sweep-benchmark")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("benchmark")

	for i := range count {
		m := sm.Metrics().AppendEmpty()
		m.SetName("request.duration")
		m.Metadata().PutInt("schema.revision", int64(i%89))
		dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(i + 1))
		dp.SetDoubleValue(float64(i))
	}
	return md
}

// BenchmarkSweepOrphansMetricMetadata measures the full ten-owner live-set
// rebuild with the metadata reference count scaled independently of its small
// distinct-value set. Sweeps are intentionally no-ops: production commonly
// reaches this path to prove that live rows must remain live.
func BenchmarkSweepOrphansMetricMetadata(b *testing.B) {
	for _, count := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("ingests=%d", count), func(b *testing.B) {
			ctx := context.Background()
			s, err := NewStore(ctx, "", zap.NewNop())
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { s.Close() })

			md := sweepBenchmarkMetrics(count)
			if err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
			}); err != nil {
				b.Fatal(err)
			}

			var metadataRefs int
			if err := s.WithDBRead(func(db *sql.DB) error {
				return db.QueryRow(`
					select count(*)
					from metric_ingests, unnest(metadata_ids)`).Scan(&metadataRefs)
			}); err != nil {
				b.Fatal(err)
			}
			if metadataRefs != count {
				b.Fatalf("metadata references = %d, want %d", metadataRefs, count)
			}

			b.ReportAllocs()
			b.ReportMetric(float64(metadataRefs), "metadata_refs")
			b.ResetTimer()
			for b.Loop() {
				if err := s.WithDBWrite(func(db *sql.DB) error {
					return ingest.SweepOrphans(ctx, db, s.FlushedIDs())
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
