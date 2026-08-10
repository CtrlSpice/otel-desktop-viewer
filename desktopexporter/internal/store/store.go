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

	// logger is never nil: NewStore substitutes a no-op when given one, so
	// call sites need no guard.
	logger *zap.Logger

	// Result of the schema version check, set once during NewStore and read
	// without locking. The warning is logged at open; this is kept so callers
	// (and the enforcement switch, when it lands) can act on the outcome
	// rather than parse a message.
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

	// 2) Check the schema version before creating anything else.
	//
	// The ordering is load-bearing, not stylistic. Run after the table and
	// index loops and an incompatible file fails inside them first --
	// `create table if not exists` leaves the old table alone, then index
	// creation dies against a column that does not exist, and the user gets
	// "failed to create index 4" instead of "this database was written by a
	// different version".
	schemaCompat, err := checkSchemaVersion(db, dbPath, logger)
	if err != nil {
		return nil, err
	}

	// 3) Create the tables for our signals
	for i, query := range schema.TableCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating table %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	// 4) Create indexes - queries use IF NOT EXISTS so reopening is safe
	for i, query := range schema.IndexCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating index %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	// 5) Create macros - queries use CREATE OR REPLACE so reopening is safe
	for i, query := range schema.MacroCreationQueries {
		if _, err = db.Exec(query); err != nil {
			return nil, fmt.Errorf("%w while creating macro %d: %w", ErrStoreInitFailed, i, err)
		}
	}

	return &Store{
		db:           db,
		conn:         conn,
		dbPath:       dbPath,
		logger:       logger,
		schemaCompat: schemaCompat,
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

// Store deliberately exposes no accessor for its *sql.DB. Handing out the pool
// would let a caller query after the lock released, which is the ordering bug
// WithDBRead and WithDBWrite exist to prevent. Callers that need the pool pass
// a closure to one of those instead.
