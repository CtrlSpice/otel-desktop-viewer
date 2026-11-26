package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestCloseWaitsForSwitchPreparation(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	dest := filepath.Join(dir, "next.duckdb")
	active := newFileStore(t, filepath.Join(dir, "original.duckdb"))
	defer active.Close()

	// Hold candidate initialization before the handoff acquires active.mu.
	schemaInitializationMu.Lock()
	release := sync.OnceFunc(schemaInitializationMu.Unlock)
	defer release()
	switched := make(chan error, 1)
	go func() {
		switched <- active.CreateDatabase(ctx, dest, func() error { return nil })
	}()
	require.Eventually(t, func() bool {
		staging, _ := filepath.Glob(filepath.Join(dir, ".viewer-init-*", "current.duckdb"))
		return len(staging) == 1
	}, 10*time.Second, time.Millisecond)

	closing, closed := make(chan struct{}), make(chan error, 1)
	go func() {
		close(closing)
		closed <- active.Close()
	}()
	<-closing
	select {
	case <-closed:
		t.Fatal("Close returned while the switch was preparing its database")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	require.NoError(t, <-switched)
	require.NoError(t, <-closed)
	require.ErrorIs(t, active.CreateDatabase(ctx, filepath.Join(dir, "late.duckdb"), func() error { return nil }), ErrStoreConnectionClosed)
	_, err := os.Stat(filepath.Join(dir, "late.duckdb"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSwitchWaitsForIngestAndKeepsEachBatchInOneFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.duckdb"), filepath.Join(dir, "second.duckdb")
	active := newFileStore(t, first)
	defer active.Close()
	entered, finish := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(finish) })
	defer release()
	written := make(chan error, 1)
	go func() {
		written <- active.WithConn(func(c driver.Conn) error {
			close(entered)
			<-finish
			return spans.Ingest(ctx, c, buildUniqueTraces(1))
		})
	}()
	<-entered
	switched := make(chan error, 1)
	go func() {
		switched <- active.CreateDatabase(ctx, second, func() error { return nil })
	}()
	// WithConn holds a read lock. Another reader is excluded only once the
	// switch is actually waiting for that in-flight batch to finish.
	require.Eventually(t, func() bool {
		if !active.mu.TryRLock() {
			return true
		}
		active.mu.RUnlock()
		return false
	}, 10*time.Second, time.Millisecond)
	release()
	require.NoError(t, <-written)
	require.NoError(t, <-switched)
	require.NoError(t, active.WithConn(func(c driver.Conn) error {
		return spans.Ingest(ctx, c, buildUniqueTraces(2))
	}))
	for i, path := range []string{first, second} {
		s := active
		if i == 0 {
			s = newFileStore(t, path)
			defer s.Close()
		}
		var ids []uint64
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.QueryContext(ctx, "select span_id from spans")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uint64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		}))
		require.Equal(t, []uint64{uint64(i + 1)}, ids)
	}
}

func TestSwitchPublishesOnlyAfterSelectionIsRemembered(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.duckdb"), filepath.Join(dir, "second.duckdb")
	active := newFileStore(t, first)
	defer active.Close()
	require.NoError(t, newFileStore(t, second).Close())
	readPath := func() string {
		t.Helper()
		var path string
		require.NoError(t, active.WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select path from duckdb_databases() where database_name = current_database()`).Scan(&path)
		}))
		return path
	}
	before, changed, _ := active.CommitSnapshot()
	refused := errors.New("selection write failed")
	require.ErrorIs(t, active.SelectDatabase(ctx, second, func() error { return refused }), refused)
	require.True(t, sameTestFile(t, first, readPath()))
	select {
	case <-changed:
		t.Fatal("failed switch invalidated the active database")
	default:
	}
	require.NoError(t, active.SelectDatabase(ctx, second, func() error { return nil }))
	require.True(t, sameTestFile(t, second, readPath()))
	after, _, closed := active.CommitSnapshot()
	require.False(t, closed)
	require.Greater(t, after, before)
	select {
	case <-changed:
	default:
		t.Fatal("successful switch did not wake Live subscribers")
	}
}

func TestSwitchUsesOneEngineForFilesWithTheSameCatalogName(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	first := filepath.Join(dir, "capture.one.duckdb")
	second := filepath.Join(dir, "capture.two.duckdb")
	active, err := OpenViewerDatabase(ctx, first, true, nil)
	require.NoError(t, err)
	defer active.Close()
	// The host table lasts only for the lifetime of this engine. Both file
	// names naturally bind as "capture"; qualified SQL must keep working.
	_, err = active.db.Exec(`create table "viewer.engine".main.engine_marker as select 17 as n;
		create table capture.main.capture_marker as select 1 as n`)
	require.NoError(t, err)
	refused := errors.New("remember failed")
	require.ErrorIs(t, active.CreateDatabase(ctx, second, func() error { return refused }), refused)
	var n int
	require.NoError(t, active.db.QueryRow(`select n from capture.main.capture_marker`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, active.SelectDatabase(ctx, second, func() error { return nil }))
	_, err = active.db.Exec(`create table capture.main.capture_marker as select 2 as n`)
	require.NoError(t, err)
	check := func(expected int) {
		t.Helper()
		var held []*sql.Conn
		defer func() {
			for _, c := range held {
				c.Close()
			}
		}()
		for range maxPoolConns {
			c, err := active.db.Conn(ctx)
			require.NoError(t, err)
			held = append(held, c)
			var name string
			var sum int
			require.NoError(t, c.QueryRowContext(ctx, `select current_database(), engine_marker.n + capture_marker.n
				from "viewer.engine".main.engine_marker, capture_marker`).Scan(&name, &sum))
			require.Equal(t, "capture", name)
			require.Equal(t, expected+17, sum)
		}
	}
	check(2)
	require.NoError(t, active.SelectDatabase(ctx, first, func() error { return nil }))
	check(1)
}

func sameTestFile(t *testing.T, a, b string) bool {
	t.Helper()
	x, err := filepath.EvalSymlinks(a)
	require.NoError(t, err)
	y, err := filepath.EvalSymlinks(b)
	require.NoError(t, err)
	return x == y
}

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
					_, err := spans.SearchTraceSummaries(ctx, db, BoundedTimeRange(0, time.Now().UnixNano()), nil)
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
					_, err := spans.SearchTraceSummaries(ctx, db, BoundedTimeRange(0, time.Now().UnixNano()), nil)
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

// TestReadsRunDuringIngest verifies that queries are served while a batch is
// appended. The bound is relative to batch time to tolerate slower CI hosts.
func TestReadsRunDuringIngest(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	// Prime, so the reader has rows to scan and the appender is warm.
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildWideTraces(0, 2000))
	}); err != nil {
		t.Fatalf("prime ingest: %v", err)
	}

	// Time one batch on its own, so the assertion below can be stated relative
	// to it rather than to a wall-clock guess.
	start := time.Now()
	if err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, buildWideTraces(1, 20000))
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
				return spans.Ingest(ctx, conn, buildWideTraces(seq, 20000))
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
				return spans.Ingest(ctx, conn, buildWideTraces(seq+100, 500))
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

// TestPoolWriteExcludesIngest prevents a sweep or prune from deleting histogram
// bounds or Metric identities while an ingestion batch is still using them.
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
				return spans.Ingest(ctx, conn, buildWideTraces(seq, 4000))
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
