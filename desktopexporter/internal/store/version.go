package store

import (
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
// Enforcement, not a warning. The check spent the rewrite in warn-only mode
// because the schema was still moving and hard failure would have meant
// deleting the dev database on every iteration. It is now what stops an
// incompatible file being opened and failing later as something opaque: an
// appender column-count error partway through an ingest, or "failed to create
// index 4" against a column that does not exist.
//
// There is no migration. The remedy is to delete the file or point --db
// somewhere else, which is what the message says.
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

	// SchemaPreVersioning means the file holds data but carries no version
	// stamp: it was written before versioning existed, so its shape is unknown.
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
func inspectSchemaVersion(db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, bool, error) {
	var hasSchemaMeta int
	if err := db.QueryRow(schema.SchemaMetaTableExistsQuery).Scan(&hasSchemaMeta); err != nil {
		return SchemaOK, false, fmt.Errorf("%w while probing for schema metadata: %w", ErrStoreInitFailed, err)
	}

	if hasSchemaMeta == 0 {
		return inspectUnstampedDatabase(db, dbPath, logger)
	}
	var metadataColumns, validMetadataColumns int
	if err := db.QueryRow(schema.SchemaMetaShapeQuery).Scan(&metadataColumns, &validMetadataColumns); err != nil {
		return SchemaOK, false, fmt.Errorf("%w: %s has malformed schema metadata: %w",
			ErrSchemaIncompatible, describePath(dbPath), err)
	}
	if metadataColumns != 1 || validMetadataColumns != 1 {
		return SchemaOK, false, malformedSchemaMetadataError(dbPath)
	}

	var count int
	var minVersion, maxVersion sql.NullInt64
	if err := db.QueryRow(schema.ReadVersionMetadataQuery).Scan(&count, &minVersion, &maxVersion); err != nil {
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

func inspectUnstampedDatabase(db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, bool, error) {
	var hasTelemetryTables int
	if err := db.QueryRow(schema.TelemetryTableExistsQuery).Scan(&hasTelemetryTables); err != nil {
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

func initializeSchemaVersion(db *sql.DB, shouldStamp bool) error {
	if !shouldStamp {
		return nil
	}
	if _, err := db.Exec(schema.VersionTableQuery); err != nil {
		return fmt.Errorf("%w while creating schema_meta: %w", ErrStoreInitFailed, err)
	}
	if _, err := db.Exec(schema.StampVersionQuery, schema.Version); err != nil {
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
