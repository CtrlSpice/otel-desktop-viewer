package spans_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// benchTraces builds a batch shaped like the reference capture: 17 attribute
// rows per span, spread over several resources and scopes, which is what
// actually drives ingest cost.
func benchTraces(spanCount int) ptrace.Traces {
	td := ptrace.NewTraces()
	const resources = 12

	for r := 0; r < resources; r++ {
		rs := td.ResourceSpans().AppendEmpty()
		res := rs.Resource().Attributes()
		res.PutStr("service.name", fmt.Sprintf("svc-%d", r))
		res.PutStr("service.version", "1.2.3")
		res.PutStr("deployment.environment", "bench")
		res.PutStr("host.name", fmt.Sprintf("host-%d", r))

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("bench.scope")
		ss.Scope().SetVersion("1.0.0")

		for i := r; i < spanCount; i += resources {
			s := ss.Spans().AppendEmpty()
			var tid [16]byte
			var sid [8]byte
			for b := 0; b < 8; b++ {
				sid[b] = byte(i >> (b * 8))
				tid[b] = byte(i >> (b * 8))
			}
			s.SetTraceID(tid)
			s.SetSpanID(sid)
			s.SetName("GET /api/v1/resource")
			s.SetKind(ptrace.SpanKindServer)
			now := time.Now()
			s.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
			s.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(3 * time.Millisecond)))

			a := s.Attributes()
			a.PutStr("http.request.method", "GET")
			a.PutInt("http.response.status_code", 200)
			a.PutStr("url.path", "/api/v1/resource")
			a.PutStr("url.scheme", "https")
			a.PutStr("server.address", "example.internal")
			a.PutInt("server.port", 8443)
			a.PutStr("network.protocol.version", "1.1")
			a.PutStr("user_agent.original", "bench/1.0")
			a.PutBool("http.request.resend", false)
			a.PutDouble("http.request.body.size", 1024)
			a.PutStr("client.address", "10.0.0.1")
		}
	}
	return td
}

// BenchmarkIngestBatch measures wall time to write one OTLP batch, which is
// what any ingest deadline has to be sized against. Sizes bracket the sending
// queue's 8192-item batch threshold.
func BenchmarkIngestBatch(b *testing.B) {
	for _, spanCount := range []int{100, 1_000, 8_192, 20_000} {
		b.Run(fmt.Sprintf("spans=%d", spanCount), func(b *testing.B) {
			ctx := context.Background()
			td := benchTraces(spanCount)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				s, err := store.NewStore(ctx, "", zap.NewNop())
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				start := time.Now()
				err = s.WithConn(func(conn driver.Conn) error {
					return spans.Ingest(ctx, conn, td)
				})
				elapsed := time.Since(start)

				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(elapsed.Milliseconds()), "ms/batch")
				s.Close()
				b.StartTimer()
			}
		})
	}
}

func BenchmarkSearchTraceSummariesLimit(b *testing.B) {
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, benchTraces(10_000))
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
					raw, queryErr = spans.SearchTraceSummariesWithOptions(ctx, db, store.BoundedTimeRange(0, 1<<63-1), nil, search.ResultOptions{Limit: &limit, Sort: bc.sort})
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

// BenchmarkGetFieldValueCompletions measures the per-keystroke cost of span-name value
// completion. The reference capture holds ~245k spans over a few hundred
// distinct names, so the bench seeds that shape directly in SQL: ingest is
// not what is being measured.
func BenchmarkGetFieldValueCompletions(b *testing.B) {
	for _, spanCount := range []int{50_000, 250_000} {
		b.Run(fmt.Sprintf("spans=%d", spanCount), func(b *testing.B) {
			ctx := context.Background()
			s, err := store.NewStore(ctx, "", zap.NewNop())
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			err = s.WithDBWrite(func(db *sql.DB) error {
				// ~400 distinct names, zipf-ish frequency via modulo.
				_, err := db.Exec(`insert into spans
					(trace_id, span_id, name, start_time, end_time,
					 resource, scope, attributes)
					select uuid(), range::ubigint,
						'svc-' || (range % 20) || '/op-' || (range % 400),
						range, range + 1,
						{'id': 'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee'::uuid,
						 'payload_id': 'dddddddd-dddd-dddd-dddd-dddddddddddd'::uuid,
						 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0},
						{'id': 'ffffffff-ffff-ffff-ffff-ffffffffffff'::uuid, 'name': 'sc', 'version': '',
						 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0}, '{}'::json::variant
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
							raw, qErr = spans.GetFieldValueCompletions(ctx, db, "name", term, 8)
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
