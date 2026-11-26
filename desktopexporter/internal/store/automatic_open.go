package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/duckdb/duckdb-go/v2"
	"go.uber.org/zap"
)

// OpenViewerDatabase reopens a recognized viewer database. Only the first
// launch may create one.
func OpenViewerDatabase(ctx context.Context, path string, createDefault bool, logger *zap.Logger) (*Store, error) {
	return CaptureFiles{}.OpenViewerDatabase(ctx, path, createDefault, logger)
}

type CaptureFiles struct {
	RecoveryDir string
}

func (files CaptureFiles) OpenViewerDatabase(ctx context.Context, path string, createDefault bool, logger *zap.Logger) (*Store, error) {
	e, err := newCaptureEngine(ctx, path)
	if err != nil {
		return nil, err
	}
	e.files = files
	if createDefault {
		if err := createViewerDatabaseOn(ctx, path, logger, e); err != nil && !errors.Is(err, os.ErrExist) {
			e.close()
			return nil, err
		}
	}
	s, err := e.openExisting(ctx, path, logger)
	if err != nil {
		e.close()
		return nil, err
	}
	s.engine = e
	return s, nil
}

func (e *captureEngine) openExisting(ctx context.Context, path string, logger *zap.Logger) (*Store, error) {
	return openFileStore(path, false, func() (*Store, error) {
		file, err := e.inspectFile(ctx, path)
		if err != nil {
			return nil, err
		}
		return openStore(ctx, path, logger, file)
	})
}

// OpenExplicitDatabase keeps --db's schema and creation rules while allowing
// the collector to switch away from its first file without closing the engine.
func OpenExplicitDatabase(ctx context.Context, path string, logger *zap.Logger) (*Store, error) {
	return CaptureFiles{}.OpenExplicitDatabase(ctx, path, logger)
}

func (files CaptureFiles) OpenExplicitDatabase(ctx context.Context, path string, logger *zap.Logger) (*Store, error) {
	e, err := newCaptureEngine(ctx, path)
	if err != nil {
		return nil, err
	}
	e.files = files
	s, err := openFileStore(path, false, func() (*Store, error) {
		f := &inspectedFile{engine: e}
		pin, err := pinUnownedFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if pin == nil {
			if err := createViewerDatabaseOn(ctx, path, logger, e); err != nil {
				return nil, err
			}
			pin, err = pinUnownedFile(path)
			if err != nil {
				return nil, err
			}
		}
		f.viewerFile = pin
		if err := f.recoverWriter(ctx); err != nil {
			return nil, errors.Join(err, f.finish(nil))
		}
		if err := pin.guard(ctx, e.control); err != nil {
			return nil, errors.Join(err, f.finish(nil))
		}
		f.guarded = true
		if err := f.attach(ctx, path, ""); err != nil {
			return nil, errors.Join(err, f.finish(nil))
		}
		return openStore(ctx, path, logger, f)
	})
	if err != nil {
		e.close()
		return nil, err
	}
	s.engine = e
	return s, nil
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
	if err := checkDuckDBPath(path); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !create {
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
	// Attachments are visible across the instance. Drain current requests
	// before the candidate catalog appears, including read-only SQL clients.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil && len(s.engine.retained) != 0 {
		return errors.New("cannot switch database: previous attachment cleanup failed; restart the collector")
	}
	if create {
		if err := createViewerDatabaseOn(ctx, path, s.logger, s.engine); err != nil {
			return err
		}
	}
	var next *Store
	var err error
	if s.engine == nil {
		next, err = OpenViewerDatabase(ctx, path, false, s.logger)
	} else {
		// Different files can have the same DuckDB catalog name (for example
		// capture.one.duckdb and capture.two.duckdb). Keep the old attachment
		// alive while leaving its ordinary name free for the candidate.
		old := s.pin
		original := old.catalog
		if err := old.rename(`"viewer.previous"`); err != nil {
			return err
		}
		defer func() {
			if s.pin == old {
				if err := old.rename(original); err != nil {
					s.logger.Error("restoring active catalog name", zap.Error(err))
				}
			}
		}()
		next, err = s.engine.openExisting(ctx, path, s.logger)
	}
	if err != nil {
		return err
	}
	defer func() {
		if err := next.Close(); err != nil {
			s.logger.Error("closing inactive database", zap.Error(err))
		}
	}()
	if s.db == nil {
		return ErrStoreConnectionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := identifyFile(path)
	if err != nil || id != next.owner.identity {
		return errors.Join(fmt.Errorf("database file changed before selection: %s", path), err)
	}
	if err := remember(); err != nil {
		return err
	}
	// No fallible operation remains between remembering the file and making
	// it visible. Close the old handles before releasing mu. Closing their
	// observer wakes subscribers, who then read the new database's revision.
	oldRevision, _, _ := s.commits.Snapshot()
	s.revisionBase += oldRevision + 1
	s.databaseHandles, next.databaseHandles = next.databaseHandles, s.databaseHandles
	if s.engine == nil {
		s.engine, next.engine = next.engine, nil
	}
	return nil
}

func (f *inspectedFile) rename(name string) error {
	if _, err := f.engine.control.Exec("alter database " + f.catalog + " set alias to " + name); err != nil {
		return err
	}
	f.catalog = name
	return nil
}

// Inspect a literal path without allowing DuckDB to create a missing file,
// recover a WAL into the file, or checkpoint it on close.
type inspectedFile struct {
	*viewerFile
	engine     *captureEngine
	guarded    bool
	attached   bool
	recognized bool
	catalog    string // quoted DuckDB identifier, derived by ATTACH
	writer     *writerRecord
}

type captureEngine struct {
	*duckdb.Connector
	control  *sql.DB
	retained []retainedFile
	files    CaptureFiles
}

type retainedFile struct {
	file  *inspectedFile
	owner *fileOwner
}

func (e *captureEngine) close() error {
	err := e.control.Close()
	for _, entry := range e.retained {
		if entry.file.viewerFile != nil {
			err = errors.Join(err, entry.file.File.Close())
		}
		if entry.file.writer != nil {
			err = errors.Join(err, entry.file.writer.release(false))
		}
		if entry.owner != nil {
			entry.owner.release()
		}
	}
	e.retained = nil
	return err
}

func newCaptureEngine(ctx context.Context, path string) (*captureEngine, error) {
	if err := checkDuckDBPath(path); err != nil {
		return nil, err
	}
	connector, err := duckdb.NewConnector(attachedDatabaseDSN(path), nil)
	if err != nil {
		return nil, err
	}
	e := &captureEngine{Connector: connector, control: sql.OpenDB(connector)}
	e.control.SetMaxOpenConns(1)
	// Leave even "memory.duckdb" free to use its normal catalog name.
	if _, err := e.control.ExecContext(ctx, `load otel_commits; alter database memory set alias to "viewer.engine"; use "viewer.engine"`); err != nil {
		e.control.Close()
		return nil, err
	}
	return e, nil
}

func (e *captureEngine) inspectFile(ctx context.Context, path string) (*inspectedFile, error) {
	file, err := pinUnownedFile(path)
	if err != nil {
		return nil, err
	}
	inspected := &inspectedFile{viewerFile: file, engine: e}
	if err := inspected.recoverWriter(ctx); err != nil {
		return nil, errors.Join(err, inspected.finish(nil))
	}
	if err := inspected.inspect(ctx, path); err != nil {
		return nil, errors.Join(err, inspected.finish(nil))
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
	return file.guardName(ctx, db, file.Name())
}

func (file *viewerFile) guardName(ctx context.Context, db *sql.DB, name string) error {
	_, err := db.ExecContext(ctx, "set variable viewer_identity = struct_pack(path := ?::varchar, device := ?::ubigint, file := ?::ubigint, fd := ?::ubigint)", name, file.device, file.id, uint64(file.Fd()))
	if err == nil {
		_, err = db.ExecContext(ctx, "set otel_resume_file = getvariable('viewer_identity')")
	}
	return err
}

func (file *inspectedFile) inspect(ctx context.Context, path string) error {
	db := file.engine.control
	if err := file.guard(ctx, db); err != nil {
		return err
	}
	file.guarded = true
	if err := file.attach(ctx, path, " (read_only)"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "use "+file.catalog); err != nil {
		return err
	}
	if err := verifyViewerDatabase(ctx, db, path); err != nil {
		return err
	}
	if _, _, err := inspectSchemaVersion(ctx, db, path, zap.NewNop(), schema.Version); err != nil {
		return err
	}
	file.recognized = true
	return file.detach()
}

func (file *inspectedFile) attach(ctx context.Context, path, options string) error {
	// Exactly one new file is attached. Ask DuckDB for its name rather than
	// guessing how this version renders platform paths or compound filenames.
	tx, err := file.engine.control.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "attach ?"+options, path); err != nil {
		return fmt.Errorf("attach %s: %w", path, err)
	}
	var name string
	if err := tx.QueryRowContext(ctx, "select database_name from duckdb_databases() where path is not null and database_name <> 'viewer.previous'").Scan(&name); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	file.catalog = `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	file.attached = true
	return nil
}

func (f *inspectedFile) detach() error {
	if !f.attached {
		return nil
	}
	_, err := f.engine.control.Exec(`use "viewer.engine"; detach ` + f.catalog)
	if err == nil {
		f.attached = false
	} else {
		// DuckDB can remove the catalog and then fail to checkpoint it.
		// Keep the checkpoint error, but release the guard if detach completed.
		var exists bool
		if probeErr := f.engine.control.QueryRow(`select exists(select 1 from duckdb_databases()
			where '"' || replace(database_name, '"', '""') || '"' = ?)`, f.catalog).Scan(&exists); probeErr != nil {
			return errors.Join(err, probeErr)
		}
		f.attached = exists
	}
	return err
}

func (f *inspectedFile) close() error {
	detachErr := f.detach()
	if f.attached {
		return detachErr
	}
	if f.guarded {
		if err := f.unguard(); err != nil {
			return errors.Join(detachErr, err)
		}
	}
	if f.writer != nil {
		detachErr = errors.Join(detachErr, f.writer.release(detachErr == nil))
		f.writer = nil
	}
	if f.viewerFile != nil {
		err := f.File.Close()
		f.viewerFile = nil
		return errors.Join(detachErr, err)
	}
	return detachErr
}

func (f *inspectedFile) unguard() error {
	name := f.Name()
	if f.writer != nil && f.writer.replaying != "" {
		name = f.writer.replaying
	}
	db := f.engine.control
	if _, err := db.Exec("set variable viewer_identity = struct_pack(path := ?::varchar, device := null::ubigint, file := null::ubigint, fd := null::ubigint)", name); err != nil {
		return err
	}
	if _, err := db.Exec("set otel_resume_file = getvariable('viewer_identity')"); err != nil {
		return err
	}
	f.guarded = false
	return nil
}

func (f *inspectedFile) finish(owner *fileOwner) error {
	err := f.close()
	if f.attached || f.guarded {
		// Keep the physical file reserved until the engine closes its remaining
		// handles. During open, the caller already holds the ownership lock.
		if owner == nil && f.viewerFile != nil {
			owner = &fileOwner{path: f.Name(), identity: f.fileIdentity}
			openFiles.owners[owner] = struct{}{}
		}
		f.engine.retained = append(f.engine.retained, retainedFile{f, owner})
	} else if owner != nil {
		owner.release()
	}
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
func createViewerDatabaseOn(ctx context.Context, path string, logger *zap.Logger, e *captureEngine) error {
	dir, err := os.MkdirTemp(filepath.Dir(path), ".viewer-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, "current.duckdb")
	var s *Store
	if e == nil {
		s, err = NewStore(ctx, staged, logger)
	} else {
		s, err = openStore(ctx, staged, logger, &inspectedFile{engine: e, catalog: `"viewer.stage"`})
	}
	if err != nil {
		return err
	}
	if err := s.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return &os.PathError{Op: "create", Path: path, Err: os.ErrExist}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := checkRecoveryFiles(path); err != nil {
		return err
	}
	if err := publishNewDatabase(staged, path); err != nil {
		return fmt.Errorf("publish complete database at %s without replacing an existing file: %w", path, err)
	}
	return nil
}

// Creating a new main file can make DuckDB discard or replay a previous
// capture's WAL. An existing recovery file belongs with its original capture.
var recoverySuffixes = [...]string{".wal", ".wal.checkpoint", ".wal.recovery"}

func checkRecoveryFiles(path string) error {
	for _, suffix := range recoverySuffixes {
		journal := path + suffix
		if _, err := os.Lstat(journal); err == nil {
			return fmt.Errorf("cannot create %s: existing recovery file %s", path, journal)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
