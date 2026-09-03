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
					return spans.Ingest(ctx, conn, traces, s.FlushedIDs())
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
					_, err := spans.SearchTraces(ctx, db, BoundedTimeRange(0, time.Now().UnixNano()), nil)
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
					_, err := spans.SearchTraces(ctx, db, BoundedTimeRange(0, time.Now().UnixNano()), nil)
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

// buildWideTraces returns one batch of n spans, so a test can make the ingest
// lock window long enough to measure.
func buildWideTraces(seq uint64, n int) ptrace.Traces {
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "stress")
	ss := rs.ScopeSpans().AppendEmpty()
	now := time.Now().UnixNano()
	for i := range n {
		sp := ss.Spans().AppendEmpty()
		var traceID [16]byte
		binary.BigEndian.PutUint64(traceID[8:], seq*uint64(n)+uint64(i))
		var spanID [8]byte
		binary.BigEndian.PutUint64(spanID[:], seq*uint64(n)+uint64(i))
		sp.SetTraceID(traceID)
		sp.SetSpanID(spanID)
		sp.SetName("wide-span")
		sp.SetStartTimestamp(pcommon.Timestamp(now + int64(i)))
		sp.SetEndTimestamp(pcommon.Timestamp(now + int64(i) + 1000))
		sp.Attributes().PutStr("driver", fmt.Sprintf("D%02d", i%20))
	}
	return tr
}

// TestReadsRunDuringIngest is the behaviour this locking scheme exists for.
//
// Ingest holds mu for reading rather than writing, so a query issued while a
// batch is being appended is served rather than queued. Before the split, a
// reader waited out the whole batch: measured at 159ms behind a 50,000-span
// batch, to perform 0.2ms of work.
//
// Asserted as a bound on the worst read rather than a speedup ratio, because a
// ratio against a moving baseline is a flaky test. The bound is loose enough
// for a loaded CI box and still far below the time one batch takes to append.
func TestReadsRunDuringIngest(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	// Prime, so the reader has rows to scan and the appender is warm.
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildWideTraces(0, 2000), s.FlushedIDs())
	}); err != nil {
		t.Fatalf("prime ingest: %v", err)
	}

	// Time one batch on its own, so the assertion below can be stated relative
	// to it rather than to a wall-clock guess.
	start := time.Now()
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildWideTraces(1, 20000), s.FlushedIDs())
	}); err != nil {
		t.Fatalf("timed ingest: %v", err)
	}
	batchTime := time.Since(start)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var ingestErr atomic.Value
	wg.Add(1)
	go func() {
		defer wg.Done()
		for seq := uint64(2); ; seq++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := s.WithConn(func(conn driver.Conn) error {
				return spans.Ingest(ctx, conn, buildWideTraces(seq, 20000), s.FlushedIDs())
			}); err != nil {
				ingestErr.Store(err)
				return
			}
		}
	}()
	// Let the replay get going, so reads land mid-batch rather than between.
	time.Sleep(150 * time.Millisecond)

	var worst time.Duration
	const reads = 30
	for range reads {
		t0 := time.Now()
		err := s.WithDBRead(func(db *sql.DB) error {
			var n int
			return db.QueryRow(`select count(*) from spans`).Scan(&n)
		})
		if err != nil {
			t.Fatalf("read during ingest: %v", err)
		}
		if d := time.Since(t0); d > worst {
			worst = d
		}
		time.Sleep(2 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	if err := ingestErr.Load(); err != nil {
		t.Fatalf("ingest failed while reads ran alongside: %v", err)
	}

	if worst > batchTime/2 {
		t.Errorf("worst read %v exceeded half a batch (%v): reads appear to be "+
			"queueing behind ingest rather than running alongside it",
			worst, batchTime)
	}
	t.Logf("batch=%v worst read=%v", batchTime, worst)
}

// TestIngestIsSerializedAgainstItself covers what mu can no longer do. Two
// ingest calls both hold it for reading, so without ingestMu they would append
// on one connection at once -- and an appender belongs to the connection that
// created it.
func TestIngestIsSerializedAgainstItself(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	var inFlight, maxInFlight atomic.Int32
	var wg sync.WaitGroup
	var failures atomic.Value
	for seq := range uint64(8) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.WithConn(func(conn driver.Conn) error {
				n := inFlight.Add(1)
				for {
					m := maxInFlight.Load()
					if n <= m || maxInFlight.CompareAndSwap(m, n) {
						break
					}
				}
				defer inFlight.Add(-1)
				return spans.Ingest(ctx, conn, buildWideTraces(seq+100, 500), s.FlushedIDs())
			})
			if err != nil {
				failures.Store(err)
			}
		}()
	}
	wg.Wait()

	if err := failures.Load(); err != nil {
		t.Fatalf("concurrent ingest failed: %v", err)
	}
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("observed %d ingest calls inside WithConn at once, want 1: the "+
			"appender connection must not be shared", got)
	}
}

// TestPoolWriteExcludesIngest is the invariant the split must not lose. Ingest
// inserts dictionary rows before flushing the owner rows that reference them,
// so a sweep or prune landing in that window would delete rows the batch is
// about to point at -- silently, since no foreign key reaches into a uuid[].
func TestPoolWriteExcludesIngest(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	var ingesting atomic.Bool
	var overlapped atomic.Bool
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for seq := uint64(200); seq < 220; seq++ {
			_ = s.WithConn(func(conn driver.Conn) error {
				ingesting.Store(true)
				defer ingesting.Store(false)
				return spans.Ingest(ctx, conn, buildWideTraces(seq, 4000), s.FlushedIDs())
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 40 {
			_ = s.WithDBWrite(func(db *sql.DB) error {
				if ingesting.Load() {
					overlapped.Store(true)
				}
				_, err := db.Exec(`delete from spans where name = 'does-not-exist'`)
				return err
			})
			time.Sleep(time.Millisecond)
		}
	}()
	wg.Wait()

	if overlapped.Load() {
		t.Error("a pool write ran while ingest was inside WithConn; clear, delete " +
			"and retention must stay exclusive with the appender")
	}
}
