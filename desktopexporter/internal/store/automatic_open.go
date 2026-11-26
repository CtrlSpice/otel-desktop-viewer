package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/duckdb/duckdb-go/v2"
	"go.uber.org/zap"
)

// OpenViewerDatabase reopens a recognized viewer database. Only the first
// launch may create one. Explicit --db uses NewStore instead.
func OpenViewerDatabase(ctx context.Context, path string, createDefault bool, logger *zap.Logger) (*Store, error) {
	if createDefault {
		if err := createViewerDatabase(ctx, path, logger); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	file, err := inspectViewerFile(ctx, path)
	if err != nil {
		return nil, err
	}
	return openStore(ctx, path, logger, file)
}

// SelectDatabase opens an existing viewer file. remember must persist the
// selection or return an error without changing it. It must not call the store.
func (s *Store) SelectDatabase(ctx context.Context, path string, remember func() error) error {
	return s.switchDatabase(ctx, path, false, remember)
}

// CreateDatabase creates and selects a viewer file without replacing one.
func (s *Store) CreateDatabase(ctx context.Context, path string, remember func() error) error {
	return s.switchDatabase(ctx, path, true, remember)
}

func (s *Store) switchDatabase(ctx context.Context, path string, create bool, remember func() error) error {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	s.mu.RLock()
	closed, active := s.db == nil, s.dbPath
	s.mu.RUnlock()
	if closed {
		return ErrStoreConnectionClosed
	}
	if active == "" || !filepath.IsAbs(path) {
		return errors.New("database selection requires an absolute file path and a file-backed store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if create {
		if err := createViewerDatabase(ctx, path, s.logger); err != nil {
			return err
		}
	} else if path == active {
		// DuckDB already holds this file's writer lock. It may still need
		// remembering if another viewer changed the selection.
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.db == nil {
			return ErrStoreConnectionClosed
		}
		if _, err := os.Stat(path); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return remember()
	}
	next, err := OpenViewerDatabase(ctx, path, false, s.logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := next.Close(); err != nil {
			s.logger.Error("closing inactive database", zap.Error(err))
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return ErrStoreConnectionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := remember(); err != nil {
		return err
	}
	// No fallible operation remains between remembering the file and making
	// it visible. Close the old handles after releasing mu. Closing their
	// observer wakes subscribers, who then read the new database's revision.
	oldRevision, _, _ := s.commits.Snapshot()
	s.revisionBase += oldRevision + 1
	s.databaseHandles, next.databaseHandles = next.databaseHandles, s.databaseHandles
	return nil
}

// Inspect a literal path without allowing DuckDB to create a missing file,
// recover a WAL into the file, or checkpoint it on close.
func inspectViewerFile(ctx context.Context, path string) (*viewerFile, error) {
	file, err := pinViewerFile(path)
	if err != nil {
		return nil, err
	}
	if err := file.inspect(ctx, path); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// Keep the OS file ID alive from inspection until the writable store closes.
// Closing an extra descriptor earlier can also release Unix process locks.
type viewerFile struct {
	*os.File
	device, id uint64
}

func (file *viewerFile) guard(ctx context.Context, conn driver.ExecerContext) error {
	_, err := conn.ExecContext(ctx, "set variable viewer_identity = struct_pack(path := ?::varchar, device := ?::ubigint, file := ?::ubigint)", []driver.NamedValue{
		{Ordinal: 1, Value: file.Name()}, {Ordinal: 2, Value: file.device}, {Ordinal: 3, Value: file.id},
	})
	if err == nil {
		_, err = conn.ExecContext(ctx, "set otel_resume_file = getvariable('viewer_identity')", nil)
	}
	return err
}

func (file *viewerFile) inspect(ctx context.Context, path string) error {
	connector, err := duckdb.NewConnector(attachedDatabaseDSN(path), func(conn driver.ExecerContext) error {
		if _, err := conn.ExecContext(ctx, "load otel_commits", nil); err != nil {
			return err
		}
		return file.guard(ctx, conn)
	})
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "attach ? as viewer_file (read_only)", path); err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, "use viewer_file"); err != nil {
		return err
	}
	if err := verifyViewerDatabase(ctx, db, path); err != nil {
		return err
	}
	_, _, err = inspectSchemaVersion(ctx, db, path, zap.NewNop(), schema.Version)
	return err
}

func verifyViewerDatabase(ctx context.Context, db *sql.DB, path string) error {
	// A generic schema_meta table alone is not a viewer signature.
	var stamps, telemetryTables int
	if err := db.QueryRowContext(ctx, schema.SchemaMetaTableExistsQuery).Scan(&stamps); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, schema.TelemetryTableExistsQuery).Scan(&telemetryTables); err != nil {
		return err
	}
	if stamps != 1 || telemetryTables == 0 {
		return fmt.Errorf("%w: %s is not a recognized viewer database; select a different database with --db", ErrSchemaIncompatible, path)
	}
	// The ordinary startup version check validates the stamp before DDL.
	return nil
}

// Initialize a private staging file first. Exclusive creation of the final
// path means that a file appearing during startup cannot be overwritten.
func createViewerDatabase(ctx context.Context, path string, logger *zap.Logger) error {
	dir, err := os.MkdirTemp(filepath.Dir(path), ".viewer-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, "current.duckdb")
	s, err := NewStore(ctx, staged, logger)
	if err != nil {
		return err
	}
	if err := s.Close(); err != nil {
		return err
	}
	// A hard link publishes the complete, closed database in one step.
	err = os.Link(staged, path)
	if err == nil || errors.Is(err, os.ErrExist) {
		return err
	}
	// Some app-data volumes do not support hard links. Still never replace
	// an existing destination on these filesystems.
	src, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return errors.Join(err, dst.Close())
}
