package spans_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.uber.org/zap"
)

func BenchmarkSearchTracesLimit(b *testing.B) {
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, benchTraces(10_000), s.FlushedIDs())
	}); err != nil {
		b.Fatal(err)
	}

	limit := int64(100)
	for _, bc := range []struct {
		name string
		sort *search.Sort
	}{
		{name: "Default"},
		{name: "ServiceNameAsc", sort: &search.Sort{Field: "serviceName", Direction: "asc"}},
		{name: "DurationDesc", sort: &search.Sort{Field: "duration", Direction: "desc"}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			var raw json.RawMessage
			for b.Loop() {
				if err := s.WithDBRead(func(db *sql.DB) error {
					var queryErr error
					raw, queryErr = spans.SearchTracesWithOptions(ctx, db, 0, 1<<63-1, nil, search.ResultOptions{Limit: &limit, Sort: bc.sort})
					return queryErr
				}); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(len(raw)), "result-B")
		})
	}
}
