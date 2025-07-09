package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/marcboeker/go-duckdb/v2"
)

type Store struct {
	db   *sql.DB
	conn driver.Conn
	mu   sync.Mutex
}

// NewStore creates a new store for the given database path.
// An empty dbPath will create a temporary in-memory database.
func NewStore(ctx context.Context, dbPath string) *Store {
	if dbPath != "" {
		dbPath = filepath.Clean(dbPath)
	}
	connector, err := duckdb.NewConnector(dbPath, nil)
	if err != nil {
		log.Fatalf(ErrInitConnector, err)
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		log.Fatalf(ErrInitConnection, err)
	}

	db := sql.OpenDB(connector)

	// 1) Create types - ignore "already exists" errors
	for i, query := range TypeCreationQueries {
		if _, err = db.Exec(query); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				log.Fatalf("failed to create type %d: %v", i, err)
			}
		}
	}

	// 2) Create the tables for our signals
	for i, query := range TableCreationQueries {
		if _, err = db.Exec(query); err != nil {
			log.Fatalf("failed to create table %d: %v", i, err)
		}
	}

	return &Store{
		db:   db,
		conn: conn,
	}
}

// Close closes the store and the underlying database connection.
// Note:
// We explicitly set the connection to nil to ensure checkConnection()
// can detect closed state because sql.DB.Close() has a graceful shutdown
// that can cause a ping to succeed briefly after close while in-progress queries finish.
func (s *Store) Close() error {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	if s.db != nil {
		s.db.Close()
		s.db = nil
	}

	return nil
}

// checkConnection verifies that the store's connection is valid.
// Returns an error if the connection is nil.
func (s *Store) checkConnection() error {
	if s.db == nil || s.conn == nil {
		return ErrStoreConnectionClosed
	}
	return nil
}
