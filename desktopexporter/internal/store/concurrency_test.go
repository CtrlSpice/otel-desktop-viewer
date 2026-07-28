package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// buildUniqueTraces returns a single-span trace whose IDs are derived from seq,
// so repeated ingest in the stress loop does not collide on primary keys.
func buildUniqueTraces(seq uint64) ptrace.Traces {
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "stress")
	ss := rs.ScopeSpans().AppendEmpty()
	sp := ss.Spans().AppendEmpty()

	var traceID [16]byte
	binary.BigEndian.PutUint64(traceID[8:], seq)
	var spanID [8]byte
	binary.BigEndian.PutUint64(spanID[:], seq)

	sp.SetTraceID(traceID)
	sp.SetSpanID(spanID)
	sp.SetName("stress-span")

	now := time.Now().UnixNano()
	sp.SetStartTimestamp(pcommon.Timestamp(now))
	sp.SetEndTimestamp(pcommon.Timestamp(now + 1000))
	return tr
}

// TestConcurrentIngestQueryAndRetention drives every path that touches the
// store at once: appender writes on the dedicated connection, SELECTs on the
// pool, and DELETE + checkpoint on the pool. Run with -race.
func TestConcurrentIngestQueryAndRetention(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	const (
		ingesters = 4
		readers   = 8
		duration  = 3 * time.Second
	)

	deadline := time.Now().Add(duration)
	var (
		wg    sync.WaitGroup
		seq   atomic.Uint64
		errCh = make(chan error, 64)
	)

	fail := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	for i := 0; i < ingesters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				traces := buildUniqueTraces(seq.Add(1))
				err := s.WithConn(func(conn driver.Conn) error {
					return spans.Ingest(ctx, conn, traces)
				})
				if err != nil {
					fail(fmt.Errorf("ingest: %w", err))
					return
				}
			}
		}()
	}

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				err := s.WithDBRead(func(db *sql.DB) error {
					_, err := spans.SearchTraces(ctx, db, 0, time.Now().UnixNano(), nil)
					return err
				})
				if err != nil {
					fail(fmt.Errorf("read: %w", err))
					return
				}
			}
		}()
	}

	// Retention: DELETE + checkpoint under the write lock. maxBytes = 1 forces
	// a prune every pass.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			if err := s.EnforceRetention(ctx, 1); err != nil {
				fail(fmt.Errorf("retention: %w", err))
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	// Clears: the JSON-RPC mutation path.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			err := s.WithDBWrite(func(db *sql.DB) error {
				return spans.Clear(ctx, db)
			})
			if err != nil {
				fail(fmt.Errorf("clear: %w", err))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestConcurrentCloseAndRead closes the store while reads are in flight. Every
// read must either complete or return ErrStoreConnectionClosed — never panic
// on a nil *sql.DB, and never trip the race detector on s.db.
func TestConcurrentCloseAndRead(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				err := s.WithDBRead(func(db *sql.DB) error {
					_, err := spans.SearchTraces(ctx, db, 0, time.Now().UnixNano(), nil)
					return err
				})
				if err != nil && !errors.Is(err, ErrStoreConnectionClosed) {
					t.Errorf("unexpected read error: %v", err)
					return
				}
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	_ = s.Close()
	wg.Wait()
}
