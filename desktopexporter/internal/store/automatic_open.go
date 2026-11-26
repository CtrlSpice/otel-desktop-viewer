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
	"strings"

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
	return openFileStore(path, false, func() (*Store, error) {
		file, err := inspectViewerFile(ctx, path)
		if err != nil {
			return nil, err
		}
		return openStore(ctx, path, logger, file)
	})
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
	closed, active, owner := s.db == nil, s.dbPath, s.owner
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
	} else {
		target, err := identifyFile(path)
		if err != nil {
			return err
		}
		if path == active && target != owner.identity {
			return fmt.Errorf("cannot select %s: database file changed", path)
		}
		if target == owner.identity {
			if err := ctx.Err(); err != nil {
				return err
			}
			if path != active {
				// Keep the original filename: its journal is named after it.
				source, err := identifyFile(active)
				if err != nil {
					return err
				}
				if source != target {
					return fmt.Errorf("cannot select %s: active database file changed", path)
				}
				return nil
			}
			return remember()
		}
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
type inspectedFile struct {
	*viewerFile
	engine  *duckdb.Connector
	catalog string // quoted DuckDB identifier, derived by ATTACH
}

func inspectViewerFile(ctx context.Context, path string) (*inspectedFile, error) {
	file, err := pinViewerFile(path)
	if err != nil {
		return nil, err
	}
	engine, err := duckdb.NewConnector(attachedDatabaseDSN(path), nil)
	if err != nil {
		file.Close()
		return nil, err
	}
	inspected := &inspectedFile{viewerFile: file, engine: engine}
	if err := inspected.inspect(ctx, path); err != nil {
		engine.Close()
		file.Close()
		return nil, err
	}
	return inspected, nil
}

// Keep the OS file ID alive from inspection until the writable store closes.
// Closing an extra descriptor earlier can also release Unix process locks.
type viewerFile struct {
	*os.File
	fileIdentity
}

func (file *viewerFile) guard(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "set variable viewer_identity = struct_pack(path := ?::varchar, device := ?::ubigint, file := ?::ubigint)", file.Name(), file.device, file.id)
	if err == nil {
		_, err = db.ExecContext(ctx, "set otel_resume_file = getvariable('viewer_identity')")
	}
	return err
}

func (file *inspectedFile) inspect(ctx context.Context, path string) error {
	// Close the inspection pool without releasing the engine or its file guard.
	db := sql.OpenDB(struct{ driver.Connector }{file.engine})
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "load otel_commits"); err != nil {
		return err
	}
	if err := file.guard(ctx, db); err != nil {
		return err
	}
	// Leave even "memory.duckdb" free to use its normal catalog name.
	if _, err := db.ExecContext(ctx, `alter database memory set alias to "viewer.engine"`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "attach ? (read_only)", path); err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	var name string
	if err := db.QueryRowContext(ctx, "select database_name from duckdb_databases() where path is not null").Scan(&name); err != nil {
		return err
	}
	file.catalog = `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	if _, err := db.ExecContext(ctx, "use "+file.catalog); err != nil {
		return err
	}
	if err := verifyViewerDatabase(ctx, db, path); err != nil {
		return err
	}
	if _, _, err := inspectSchemaVersion(ctx, db, path, zap.NewNop(), schema.Version); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `use "viewer.engine"; detach `+file.catalog)
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
