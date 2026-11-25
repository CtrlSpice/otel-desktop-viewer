package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
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
	if err := inspectViewerFile(ctx, path); err != nil {
		return nil, err
	}
	return openStore(ctx, path, logger, true)
}

// Inspect a literal path without allowing DuckDB to create a missing file,
// recover a WAL into the file, or checkpoint it on close.
func inspectViewerFile(ctx context.Context, path string) error {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return err
	}
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
