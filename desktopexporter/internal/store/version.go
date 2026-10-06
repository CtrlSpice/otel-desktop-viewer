package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"go.uber.org/zap"
)

// ErrSchemaIncompatible is returned when the database on disk was written by a
// different schema than this build understands.
//
// The store rejects incompatible files before running application DDL. There
// is no migration; callers must delete the file or select another database.
var ErrSchemaIncompatible = errors.New("database schema is incompatible with this build")

// schemaInitializationMu keeps concurrent first opens in this process from
// both deciding that an unstamped database is fresh.
var schemaInitializationMu sync.Mutex

// SchemaCompatibility describes what the version check found when the store was
// opened.
type SchemaCompatibility int

const (
	// SchemaOK means the file carries the version this build writes, or was
	// brand new and has just been stamped with it.
	SchemaOK SchemaCompatibility = iota

	// SchemaPreVersioning means the file holds data without a version stamp, so
	// its shape is unknown.
	SchemaPreVersioning

	// SchemaMismatch means the file is stamped with a different version.
	SchemaMismatch
)

// SchemaCompatibility reports what the version check found. SchemaOK for an
// in-memory store, which is always created fresh.
func (s *Store) SchemaCompatibility() SchemaCompatibility {
	return s.schemaCompat
}

// inspectSchemaVersion reads only the catalog and version metadata before any
// application DDL runs. The returned bool says whether a verified fresh store
// needs its initial stamp.
func inspectSchemaVersion(ctx context.Context, db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, bool, error) {
	var hasSchemaMeta int
	if err := db.QueryRowContext(ctx, schema.SchemaMetaTableExistsQuery).Scan(&hasSchemaMeta); err != nil {
		return SchemaOK, false, fmt.Errorf("%w while probing for schema metadata: %w", ErrStoreInitFailed, err)
	}

	if hasSchemaMeta == 0 {
		return inspectUnstampedDatabase(ctx, db, dbPath, logger)
	}
	var metadataColumns, validMetadataColumns int
	if err := db.QueryRowContext(ctx, schema.SchemaMetaShapeQuery).Scan(&metadataColumns, &validMetadataColumns); err != nil {
		return SchemaOK, false, fmt.Errorf("%w: %s has malformed schema metadata: %w",
			ErrSchemaIncompatible, describePath(dbPath), err)
	}
	if metadataColumns != 1 || validMetadataColumns != 1 {
		return SchemaOK, false, malformedSchemaMetadataError(dbPath)
	}

	var count int
	var minVersion, maxVersion sql.NullInt64
	if err := db.QueryRowContext(ctx, schema.ReadVersionMetadataQuery).Scan(&count, &minVersion, &maxVersion); err != nil {
		return SchemaOK, false, fmt.Errorf("%w: %s has malformed schema metadata: %w",
			ErrSchemaIncompatible, describePath(dbPath), err)
	}
	if count != 1 || !minVersion.Valid || !maxVersion.Valid || minVersion.Int64 != maxVersion.Int64 {
		return SchemaOK, false, malformedSchemaMetadataError(dbPath)
	}
	if maxVersion.Int64 == schema.Version {
		return SchemaOK, false, nil
	}
	logger.Error("database was written by a different schema version",
		zap.String("database", describePath(dbPath)),
		zap.Int64("file_version", maxVersion.Int64),
		zap.Int("expected_version", schema.Version),
		zap.String("remedy", "delete it or pass a different --db path"))
	return SchemaMismatch, false, fmt.Errorf("%w: %s was written by schema version %d, "+
		"this build uses %d -- delete it or pass a different --db path",
		ErrSchemaIncompatible, describePath(dbPath), maxVersion.Int64, schema.Version)
}

func inspectUnstampedDatabase(ctx context.Context, db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, bool, error) {
	var hasTelemetryTables int
	if err := db.QueryRowContext(ctx, schema.TelemetryTableExistsQuery).Scan(&hasTelemetryTables); err != nil {
		return SchemaOK, false, fmt.Errorf("%w while probing for existing tables: %w", ErrStoreInitFailed, err)
	}
	if hasTelemetryTables != 0 {
		logger.Error("database holds data but carries no schema version, so it predates "+
			"versioning and its shape cannot be confirmed",
			zap.String("database", describePath(dbPath)),
			zap.Int("expected_version", schema.Version),
			zap.String("remedy", "delete it or pass a different --db path"))
		return SchemaPreVersioning, false, fmt.Errorf("%w: %s holds telemetry tables but carries no schema "+
			"version, so it predates versioning -- delete it or pass a different --db path",
			ErrSchemaIncompatible, describePath(dbPath))
	}
	return SchemaOK, true, nil
}

func initializeSchemaVersion(ctx context.Context, db *sql.DB, shouldStamp bool) error {
	if !shouldStamp {
		return nil
	}
	if _, err := db.ExecContext(ctx, schema.VersionTableQuery); err != nil {
		return fmt.Errorf("%w while creating schema_meta: %w", ErrStoreInitFailed, err)
	}
	if _, err := db.ExecContext(ctx, schema.StampVersionQuery, schema.Version); err != nil {
		return fmt.Errorf("%w while stamping schema version: %w", ErrStoreInitFailed, err)
	}
	return nil
}

func malformedSchemaMetadataError(dbPath string) error {
	return fmt.Errorf("%w: %s has malformed schema metadata -- delete it or pass a different --db path",
		ErrSchemaIncompatible, describePath(dbPath))
}

func describePath(dbPath string) string {
	if dbPath == "" {
		return "(in-memory)"
	}
	return dbPath
}
