package spans_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.uber.org/zap"
)

// BenchmarkGetFieldValues measures the per-keystroke cost of span-name value
// completion. The reference capture holds ~245k spans over a few hundred
// distinct names, so the bench seeds that shape directly in SQL: ingest is
// not what is being measured.
func BenchmarkGetFieldValues(b *testing.B) {
	for _, spanCount := range []int{50_000, 250_000} {
		b.Run(fmt.Sprintf("spans=%d", spanCount), func(b *testing.B) {
			ctx := context.Background()
			s, err := store.NewStore(ctx, "", zap.NewNop())
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			err = s.WithDBWrite(func(db *sql.DB) error {
				if _, err := db.Exec(`insert into resources (id, attribute_ids)
					values ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', [])`); err != nil {
					return err
				}
				if _, err := db.Exec(`insert into scopes (id, name, version, attribute_ids)
					values ('ffffffff-ffff-ffff-ffff-ffffffffffff', 'sc', '', [])`); err != nil {
					return err
				}
				// ~400 distinct names, zipf-ish frequency via modulo.
				_, err := db.Exec(`insert into spans
					(trace_id, span_id, name, start_time, end_time,
					 resource_id, scope_id, attribute_ids)
					select uuid(), range::ubigint,
						'svc-' || (range % 20) || '/op-' || (range % 400),
						range, range + 1,
						'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
						'ffffffff-ffff-ffff-ffff-ffffffffffff', []
					from range(?)`, spanCount)
				return err
			})
			if err != nil {
				b.Fatal(err)
			}

			for _, term := range []string{"", "op-1", "svc-7/op"} {
				b.Run(fmt.Sprintf("term=%q", term), func(b *testing.B) {
					for b.Loop() {
						var raw json.RawMessage
						err := s.WithDBRead(func(db *sql.DB) error {
							var qErr error
							raw, qErr = spans.GetFieldValues(ctx, db, "name", term, 8)
							return qErr
						})
						if err != nil || len(raw) == 0 {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
