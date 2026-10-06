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

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/duckdb/duckdb-go/v2"
	"go.uber.org/zap"
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

	// mu protects db and conn. Queries and ingest hold it for reading so they
	// may overlap. Pool mutations and Close hold it for writing and exclude
	// ingest; otherwise a sweep could delete dictionary rows before ingest writes
	// their owners. Store locking methods are not reentrant.
	mu sync.RWMutex

	// ingestMu serializes appender use on the dedicated ingest connection.
	ingestMu sync.Mutex

	// flushed records which dictionary rows this store has already written, so
	// a batch whose attributes, resource and scope are all known can skip its
	// insert. Owned here because the set describes one database: sharing it
	// across stores would skip inserts into the wrong one. Warmed from disk in
	// NewStore via ingest.LoadFlushedIDs, so a persistent --db that already
	// holds the dictionary does not re-insert everything until the cache
	// refills. Invalidated by ingest.SweepOrphans, the only thing that deletes
	// dictionary rows.
	flushed *ingest.FlushedIDs

	// retentionCapBytes is the store size cap enforced by EnforceRetention
	// and reported by getStats. 0 means retention is disabled. Set once via
	// SetRetentionCap before the store is shared; read without locking.
	retentionCapBytes int64

	// logger is never nil: NewStore substitutes a no-op when given one, so
	// call sites need no guard.
	logger *zap.Logger

	// Result of the accepted schema version check, set once during NewStore and
	// read without locking.
	schemaCompat SchemaCompatibility
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
//
// logger may be nil, in which case nothing is logged.
func NewStore(ctx context.Context, dbPath string, logger *zap.Logger) (*Store, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

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

	// Keep idle connections because queries hold no connection-local state.
	db.SetMaxOpenConns(maxPoolConns)
	db.SetMaxIdleConns(maxPoolConns)
	db.SetConnMaxIdleTime(5 * time.Minute)
	initialized := false
	defer func() {
		if !initialized {
			_ = db.Close()
			_ = conn.Close()
		}
	}()
	schemaInitializationMu.Lock()
	defer schemaInitializationMu.Unlock()

	// Inspect before any application DDL so refusing an incompatible store is
	// read-only. Only a verified fresh store receives the metadata stamp below.
	schemaCompat, shouldStamp, err := inspectSchemaVersion(ctx, db, dbPath, logger)
	if err != nil {
		return nil, err
	}
	// Create types; DuckDB has no IF NOT EXISTS for these statements.
	for _, stmt := range queries.Types() {
		if _, err = db.ExecContext(ctx, stmt.SQL); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return nil, fmt.Errorf("%w while creating type %s: %w", ErrStoreInitFailed, stmt.Name, err)
			}
		}
	}

	// Stamp version metadata only for a verified fresh store.
	if err := initializeSchemaVersion(ctx, db, shouldStamp); err != nil {
		return nil, err
	}

	// Create signal tables.
	for _, stmt := range queries.Tables() {
		if _, err = db.ExecContext(ctx, stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating table %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// Create indexes. Queries use IF NOT EXISTS so reopening is safe.
	for _, stmt := range queries.Indexes() {
		if _, err = db.ExecContext(ctx, stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating index %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// Create macros. Queries use CREATE OR REPLACE so reopening is safe.
	for _, stmt := range queries.Macros() {
		if _, err = db.ExecContext(ctx, stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating macro %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// Warm the dictionary cache with every retained ID.
	flushed, err := ingest.LoadFlushedIDs(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("%w while warming the dictionary flush cache: %w", ErrStoreInitFailed, err)
	}

	initialized = true
	return &Store{
		db:           db,
		conn:         conn,
		dbPath:       dbPath,
		logger:       logger,
		schemaCompat: schemaCompat,
		flushed:      flushed,
	}, nil
}

// FlushedIDs is the store's record of which dictionary rows are already
// written. Pass it to spans.Ingest, logs.Ingest and metrics.Ingest so a batch
// whose content has all been seen can skip its dictionary insert.
//
// Passing nil instead is always safe -- it just reinserts every time -- but
// passing *another* store's set is not, which is why this hangs off the store
// rather than being a package-level singleton.
func (s *Store) FlushedIDs() *ingest.FlushedIDs {
	return s.flushed
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

// WithConn runs fn against the store's dedicated appender connection. Ingest
// uses this: DuckDB appenders are bound to the connection that created them, so
// ingest cannot run on the pool.
//
// Takes ingestMu to serialize against other ingest calls, then the *read* lock,
// which lets queries run while a batch is being appended while still excluding
// pool mutations and Close. See the note on mu.
//
// Lock order is ingestMu then mu, and nothing acquires them the other way
// round, so the pair cannot deadlock.
//
// No transaction here: ingest opens one per bisection attempt, and DuckDB has
// no nested transactions. See ingest.InTransaction.
func (s *Store) WithConn(fn func(conn driver.Conn) error) error {
	s.ingestMu.Lock()
	defer s.ingestMu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil || s.conn == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.conn)
}

// WithDBRead runs fn against the connection pool under the read lock. Use it
// for SELECTs. Concurrent readers run in parallel, and run alongside ingest;
// they never overlap a pool mutation or Close.
func (s *Store) WithDBRead(fn func(db *sql.DB) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.db)
}

// WithDBWrite runs fn against the connection pool under the write lock. Use it
// for DELETE, checkpoint, and anything else that mutates. The write lock
// excludes ingest, which holds mu for reading, so a pool mutation still cannot
// race the appender writes ingest performs on a different connection.
func (s *Store) WithDBWrite(fn func(db *sql.DB) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.db)
}

// Store deliberately exposes no accessor for its *sql.DB. Handing out the pool
// would let a caller query after the lock released, which is the ordering bug
// WithDBRead and WithDBWrite exist to prevent. Callers that need the pool pass
// a closure to one of those instead.
