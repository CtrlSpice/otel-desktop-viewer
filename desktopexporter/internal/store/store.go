package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/duckdb/duckdb-go/v2"
)

// maxPoolConns caps the *sql.DB pool. Reads run concurrently under the store's
// read lock, so several DuckDB connections can be live at once; without a cap
// a burst of JSON-RPC calls opens one per in-flight request, and each is a real
// DuckDB connection with real memory cost.
const maxPoolConns = 4

// Sentinel errors for use with errors.Is.
var (
	ErrStoreConnectionClosed = errors.New("store connection is closed")
	ErrStoreInitFailed       = errors.New("store initialization failed")
)

type Store struct {
	db     *sql.DB
	conn   driver.Conn
	dbPath string // empty means in-memory mode

	// mu orders access to both handles. The write lock is shared by appender
	// writes (WithConn), pool mutations (WithDBWrite), and Close, so those are
	// mutually exclusive even though they run on different connections. Reads
	// (WithDBRead) run concurrently with each other and never overlap a write.
	//
	// Not reentrant: never call a locking Store method from inside a WithConn,
	// WithDBRead, or WithDBWrite callback.
	mu sync.RWMutex

	// retentionCapBytes is the store size cap enforced by EnforceRetention
	// and reported by getStats. 0 means retention is disabled. Set once via
	// SetRetentionCap before the store is shared; read without locking.
	retentionCapBytes int64
}

// SetRetentionCap sets the store size cap in bytes. Call before the store is
// shared across goroutines.
func (s *Store) SetRetentionCap(bytes int64) {
	s.retentionCapBytes = bytes
}

// RetentionCap returns the store size cap in bytes; 0 means disabled.
func (s *Store) RetentionCap() int64 {
	return s.retentionCapBytes
}

// NewStore creates a new store for the given database path.
// An empty dbPath will create a temporary in-memory database.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	if dbPath != "" {
		dbPath = filepath.Clean(dbPath)
	}
	connector, err := duckdb.NewConnector(dbPath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}

	db := sql.OpenDB(connector)

	// Idle connections are kept rather than dropped: we hold no connection-local
	// temporary state, so reusing them is safe, and steady-state UI polling would
	// otherwise reopen DuckDB connections on every request.
	db.SetMaxOpenConns(maxPoolConns)
	db.SetMaxIdleConns(maxPoolConns)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// 1) Create types - ignore "already exists" errors
	for i, query := range schema.TypeCreationQueries {
		if _, err = db.Exec(query); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return nil, fmt.Errorf("%w while creating type %d: %w", ErrStoreInitFailed, i, err)
			}
		}
	}

	// 2) Create the tables for our signals
	for i, query := range schema.TableCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating table %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	// 3) Create indexes - queries use IF NOT EXISTS so reopening is safe
	for i, query := range schema.IndexCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating index %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	// 4) Create macros - queries use CREATE OR REPLACE so reopening is safe
	for i, query := range schema.MacroCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating macro %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	return &Store{
		db:     db,
		conn:   conn,
		dbPath: dbPath,
	}, nil
}

// Close closes the store and the underlying database connection.
// It acquires the mutex to avoid racing with WithConn.
// We explicitly set the connection to nil so that WithConn detects the
// closed state, because sql.DB.Close() has a graceful shutdown that can
// cause a ping to succeed briefly after close.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var connErr, dbErr error
	if s.conn != nil {
		connErr = s.conn.Close()
		s.conn = nil
	}

	if s.db != nil {
		dbErr = s.db.Close()
		s.db = nil
	}

	return errors.Join(connErr, dbErr)
}

// WithConn runs fn against the store's dedicated appender connection under the
// write lock. Ingest uses this: DuckDB appenders are bound to the connection
// that created them, so ingest cannot run on the pool.
func (s *Store) WithConn(fn func(conn driver.Conn) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil || s.conn == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.conn)
}

// WithDBRead runs fn against the connection pool under the read lock. Use it
// for SELECTs. Concurrent readers run in parallel and never overlap a writer.
func (s *Store) WithDBRead(fn func(db *sql.DB) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.db)
}

// WithDBWrite runs fn against the connection pool under the write lock. Use it
// for DELETE, checkpoint, and anything else that mutates. Sharing the write
// lock with WithConn is the point: it keeps pool mutations from racing the
// appender writes that ingest performs on a different connection.
func (s *Store) WithDBWrite(fn func(db *sql.DB) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.db)
}

// DB returns the underlying *sql.DB without holding the lock across the
// caller's work.
//
// Test-only. Production code must use WithDBRead or WithDBWrite so access is
// ordered against ingest, retention, and Close. Tests use one store per test
// and drive it from a single goroutine, where that ordering is not needed.
func (s *Store) DB() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}
