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

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/duckdb/duckdb-go/v2"
)

// Sentinel errors for use with errors.Is.
var (
	ErrStoreConnectionClosed = errors.New("store connection is closed")
	ErrStoreInitFailed       = errors.New("store initialization failed")
)

type Store struct {
	db   *sql.DB
	conn driver.Conn
	mu   sync.Mutex
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

	return &Store{
		db:   db,
		conn: conn,
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

// WithConn locks the store, verifies the connection is alive,
// and passes it to fn. The lock is released when fn returns.
func (s *Store) WithConn(fn func(conn driver.Conn) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil || s.conn == nil {
		return ErrStoreConnectionClosed
	}

	return fn(s.conn)
}

// DB returns the underlying *sql.DB. Used by subpackages and tests.
func (s *Store) DB() *sql.DB {
	return s.db
}
