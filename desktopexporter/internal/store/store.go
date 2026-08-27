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
	ErrStoreTransaction      = errors.New("store transaction failed")
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
	// mu orders access to db and conn. Read-held by queries *and by ingest*,
	// write-held by pool mutations and Close.
	//
	// Ingest holding the read lock is the point. Appending on the dedicated
	// connection and querying on the pool are two DuckDB connections to one
	// database, and DuckDB's MVCC serves a reader alongside a writer without
	// help from us -- verified by running pooled SELECTs against a continuous
	// 20k-span appender ingest with no lock at all: 1,225 reads, no failures,
	// no races. Excluding readers for the duration of a batch was therefore
	// costing latency to buy nothing, and the cost scaled with batch size: a
	// reader waited 159ms behind a 50,000-span batch to perform 0.2ms of work.
	//
	// What still must be exclusive is ingest against *pool mutations* -- clear,
	// delete, retention prune and checkpoint -- and that is what the write lock
	// now means. The sharpest reason is the orphan sweep: ingest inserts
	// dictionary rows before flushing the owner rows that reference them, so a
	// sweep interleaved in that window would delete rows the in-flight batch is
	// about to point at. No error, no failed constraint, just attributes
	// quietly missing.
	mu sync.RWMutex

	// ingestMu serializes ingest against itself. mu cannot: two ingest calls
	// both hold it for reading, and a DuckDB appender belongs to the one
	// connection that made it.
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
	for _, stmt := range queries.Types() {
		if _, err = db.Exec(stmt.SQL); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return nil, fmt.Errorf("%w while creating type %s: %w", ErrStoreInitFailed, stmt.Name, err)
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
	for _, stmt := range queries.Tables() {
		if _, err = db.Exec(stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating table %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// 4) Create indexes - queries use IF NOT EXISTS so reopening is safe
	for _, stmt := range queries.Indexes() {
		if _, err = db.Exec(stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating index %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// 5) Create macros - queries use CREATE OR REPLACE so reopening is safe
	for _, stmt := range queries.Macros() {
		if _, err = db.Exec(stmt.SQL); err != nil {
			return nil, fmt.Errorf("%w while creating macro %s: %w", ErrStoreInitFailed, stmt.Name, err)
		}
	}

	// 6) Warm the dictionary flush cache from whatever is already on disk.
	// Without this, a persistent --db reopened with its dictionary intact
	// would still re-insert every attribute, resource and scope until enough
	// batches had flushed to refill an empty in-process cache -- exactly the
	// fixed per-batch cost FlushedIDs exists to remove, paid again on every
	// restart. Loads every id ever stored, which is bounded the same way the
	// on-disk dictionary itself is: by retention.
	flushed, err := ingest.LoadFlushedIDs(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("%w while warming the dictionary flush cache: %w", ErrStoreInitFailed, err)
	}

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
// No transaction is opened here. Ingest owns its own, per appender pass,
// because a failed pass is retried in narrowing halves to isolate the rows
// that caused it -- and DuckDB has neither nested transactions ("cannot start
// a transaction within a transaction") nor SAVEPOINT, so each attempt has to
// be top level. See ingest.InTransaction.
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
