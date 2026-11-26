package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/commitbridge"
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

// databaseHandles move together when the active file changes.
type databaseHandles struct {
	db           *sql.DB
	conn         driver.Conn
	dbPath       string // empty means in-memory mode
	pin          *inspectedFile
	owner        *fileOwner
	commits      *commitbridge.Observer
	schemaCompat SchemaCompatibility
}

type Store struct {
	databaseHandles
	engine *captureEngine // outlives the pools for individual attached files
	// A switch prepares the next file while this store still serves requests.
	// Serialize switches; mu guards the shorter handoff and all database access.
	switchMu     sync.Mutex
	revisionBase uint64
	inMemory     bool // Only file-backed stores can switch databases.

	// mu protects db and conn. Queries and ingest hold it for reading so they
	// may overlap. Pool mutations and Close hold it for writing and exclude
	// ingest; otherwise a sweep could delete histogram bounds before ingest writes
	// their datapoints. Store locking methods are not reentrant.
	mu sync.RWMutex

	// ingestMu serializes appender use on the dedicated ingest connection.
	ingestMu sync.Mutex

	// retentionCapBytes is the store size cap enforced by EnforceRetention
	// and reported by getStats. 0 means retention is disabled. Set once via
	// SetRetentionCap before the store is shared; read without locking.
	retentionCapBytes int64

	// logger is never nil: NewStore substitutes a no-op when given one, so
	// call sites need no guard.
	logger *zap.Logger
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
	if dbPath == "" {
		return openStore(ctx, "", logger, nil)
	}
	path, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	if err := checkDuckDBPath(path); err != nil {
		return nil, err
	}
	return openFileStore(path, !needsLiteralAttach(path), func() (*Store, error) {
		return openStore(ctx, path, logger, nil)
	})
}

// DuckDB treats backslashes as separators even on Unix and interprets a
// question mark in a path when deriving auxiliary filenames such as the WAL.
func checkDuckDBPath(path string) error {
	if filepath.Separator != '\\' && strings.Contains(path, `\`) {
		return fmt.Errorf("DuckDB cannot open this literal path: backslash is interpreted as a directory separator: %s", path)
	}
	if strings.Contains(path, "?") {
		return fmt.Errorf("DuckDB cannot open this literal path: question mark in database path: %s", path)
	}
	return nil
}

// openStore takes ownership of pin on both success and failure.
func openStore(ctx context.Context, dbPath string, logger *zap.Logger, inspected *inspectedFile) (*Store, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	if dbPath != "" {
		dbPath = filepath.Clean(dbPath)
	}
	commits := commitbridge.New()
	initialized := false
	defer func() {
		if !initialized {
			commits.Close()
			if inspected != nil {
				inspected.close()
			}
		}
	}()
	connector, err := storeConnector(ctx, dbPath, commits, inspected)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		_ = connector.Close()
		return nil, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}

	db := sql.OpenDB(connector)

	// Reuse initialized connections. For attached files, each connection
	// selects the capture's catalog before entering the pool.
	db.SetMaxOpenConns(maxPoolConns)
	db.SetMaxIdleConns(maxPoolConns)
	db.SetConnMaxIdleTime(5 * time.Minute)
	defer func() {
		if !initialized {
			_ = db.Close()
			_ = conn.Close()
		}
	}()
	if inspected != nil && inspected.recognized {
		// Verify the writable connection as well: the path may have changed
		// since the read-only inspection.
		if err := verifyViewerDatabase(ctx, db, dbPath); err != nil {
			return nil, err
		}
	}
	if err := arrowbridge.Load(ctx, conn); err != nil {
		return nil, fmt.Errorf("%w while loading Arrow ingestion: %w", ErrStoreInitFailed, err)
	}
	schemaInitializationMu.Lock()
	defer schemaInitializationMu.Unlock()

	migrations, err := schema.Migrations()
	if err != nil {
		return nil, fmt.Errorf("%w while loading migrations: %w", ErrStoreInitFailed, err)
	}
	// Viewer captures are often smaller than DuckDB's default 30,000-row
	// shredding threshold. Make every row group eligible at checkpoint.
	if _, err := db.ExecContext(ctx, "set variant_minimum_shredding_size = 0"); err != nil {
		return nil, fmt.Errorf("%w while enabling small-row-group VARIANT shredding: %w", ErrStoreInitFailed, err)
	}
	schemaCompat, err := initializeSchema(ctx, db, dbPath, logger, schema.Version, migrations)
	if err != nil {
		return nil, err
	}

	initialized = true
	s := &Store{
		databaseHandles: databaseHandles{db: db, conn: conn, commits: commits, pin: inspected, dbPath: dbPath, schemaCompat: schemaCompat},
		logger:          logger,
		inMemory:        dbPath == "",
	}
	return s, nil
}

// An in-memory connector will attach the file, so give it the same spill
// directory DuckDB uses when it opens that file directly.
func attachedDatabaseDSN(path string) string {
	return "?temp_directory=" + url.QueryEscape(path+".tmp")
}

func needsLiteralAttach(path string) bool {
	// duckdb-go parses the DSN as a URL, but opens the original bytes.
	// Encoding '%' as "%25" would therefore change the filename on disk.
	_, parseErr := url.Parse(path)
	return parseErr != nil && strings.Contains(path, "%") && !strings.Contains(path, "?")
}

type storeDriverConnector interface {
	driver.Connector
	Close() error
}

type initializedConnector struct {
	driver.Connector
	init func(driver.ExecerContext) error
}

// The store, not a capture's connection pool, owns the shared engine.
func (c initializedConnector) Close() error { return nil }

func (c initializedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.init(conn.(driver.ExecerContext)); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

func storeConnector(ctx context.Context, path string, commits *commitbridge.Observer, inspected *inspectedFile) (storeDriverConnector, error) {
	if inspected != nil {
		catalog := inspected.catalog
		if !inspected.attached {
			_, err := inspected.engine.control.ExecContext(ctx, "attach ? as "+catalog, path)
			if err != nil {
				return nil, err
			}
			inspected.attached = true
		}
		return initializedConnector{inspected.engine, func(conn driver.ExecerContext) error {
			if err := commits.Init(conn); err != nil {
				return err
			}
			_, err := conn.ExecContext(context.Background(), "use "+catalog, nil)
			return err
		}}, nil
	}
	if !needsLiteralAttach(path) {
		return duckdb.NewConnector(path, commits.Init)
	}
	var attach sync.Once
	var attachErr error
	return duckdb.NewConnector(attachedDatabaseDSN(path), func(conn driver.ExecerContext) error {
		if err := commits.Init(conn); err != nil {
			return err
		}
		attach.Do(func() {
			arg := []driver.NamedValue{{Ordinal: 1, Value: path}}
			_, attachErr = conn.ExecContext(ctx, "attach ? as viewer_file", arg)
		})
		if attachErr != nil {
			return attachErr
		}
		_, err := conn.ExecContext(context.Background(), "use viewer_file", nil)
		return err
	})
}

// Close closes the store and the underlying database connection.
// It acquires the mutex to avoid racing with WithConn.
// We explicitly set the connection to nil so that WithConn detects the
// closed state, because sql.DB.Close() has a graceful shutdown that can
// cause a ping to succeed briefly after close.
func (s *Store) Close() error {
	// A switch owns its candidate until it has closed the inactive handles.
	s.switchMu.Lock()
	defer s.switchMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.databaseHandles.close()
	if s.engine != nil {
		err = errors.Join(err, s.engine.control.Close())
		s.engine = nil
	}
	return err
}

func (s *databaseHandles) close() error {
	var connErr, dbErr error
	if s.conn != nil {
		connErr = s.conn.Close()
		s.conn = nil
	}

	if s.db != nil {
		dbErr = s.db.Close()
		s.db = nil
	}
	if s.commits != nil {
		s.commits.Close()
	}

	var pinErr error
	if s.pin != nil {
		pinErr = s.pin.close()
		s.pin = nil
	}
	if s.owner != nil {
		s.owner.release()
		s.owner = nil
	}
	return errors.Join(connErr, dbErr, pinErr)
}

// CommitSnapshot reports writes committed by any store connection, including
// retention and successful ingestion sub-batches. It performs no database work.
func (s *Store) CommitSnapshot() (uint64, <-chan struct{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	revision, changed, closed := s.commits.Snapshot()
	return s.revisionBase + revision, changed, closed
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
