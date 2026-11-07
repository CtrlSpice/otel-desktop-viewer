package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// Compare the full Arrow chart query, including the coarser sparklines, with
// identical M4 settings for continuous series and series with an interruption.
func BenchmarkGaugeView(b *testing.B) {
	for _, interrupted := range []bool{false, true} {
		b.Run(strconv.FormatBool(interrupted), func(b *testing.B) {
			ctx := context.Background()
			s, err := store.NewStore(ctx, "", zap.NewNop())
			require.NoError(b, err)
			b.Cleanup(func() { s.Close() })
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("benchmark.gauge")
			points := m.SetEmptyGauge().DataPoints()
			base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano()
			for series := range 10 {
				for second := range 5000 {
					if interrupted && second >= 2000 && second < 3000 {
						continue
					}
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.Timestamp(base + int64(second)*int64(time.Second)))
					p.SetIntValue(int64(second % 97))
					p.Attributes().PutInt("series", int64(series))
				}
			}
			require.NoError(b, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id, err := readStore(s, func(db *sql.DB) (string, error) {
				var id string
				err := db.QueryRowContext(ctx, "select id::varchar from metrics where name = 'benchmark.gauge'").Scan(&id)
				return id, err
			})
			require.NoError(b, err)
			query := func() {
				_, err := readStore(s, func(db *sql.DB) (int, error) {
					result, err := metrics.ViewArrow(ctx, db, id, store.BoundedTimeRange(base, base+5000*int64(time.Second)-1),
						128, nil, nil, 0, 120, 64, nil, "", nil, 0, false, "")
					if err != nil {
						return 0, err
					}
					defer result.Close()
					return 0, result.Write(ctx, io.Discard)
				})
				require.NoError(b, err)
			}
			query()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				query()
			}
		})
	}
}
