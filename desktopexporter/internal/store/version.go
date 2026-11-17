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
// schema outside this build's supported migration range, or has invalid metadata.
var ErrSchemaIncompatible = errors.New("database schema is incompatible with this build")

// schemaInitializationMu serializes inspection and initialization/migration.
var schemaInitializationMu sync.Mutex

// SchemaCompatibility describes what the version check found when the store was
// opened.
type SchemaCompatibility int

const (
	// SchemaOK means the store was accepted, including creation or migration
	// to the version this build writes.
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
// application DDL runs. Version zero denotes a verified fresh store.
func inspectSchemaVersion(ctx context.Context, db *sql.DB, dbPath string, logger *zap.Logger, target int) (SchemaCompatibility, int, error) {
	var hasSchemaMeta int
	if err := db.QueryRowContext(ctx, schema.SchemaMetaTableExistsQuery).Scan(&hasSchemaMeta); err != nil {
		return SchemaOK, 0, fmt.Errorf("%w while probing for schema metadata: %w", ErrStoreInitFailed, err)
	}

	if hasSchemaMeta == 0 {
		return inspectUnstampedDatabase(ctx, db, dbPath, logger)
	}
	var metadataColumns, validMetadataColumns int
	if err := db.QueryRowContext(ctx, schema.SchemaMetaShapeQuery).Scan(&metadataColumns, &validMetadataColumns); err != nil {
		return SchemaOK, 0, fmt.Errorf("%w: %s has malformed schema metadata: %w",
			ErrSchemaIncompatible, describePath(dbPath), err)
	}
	if metadataColumns != 1 || validMetadataColumns != 1 {
		return SchemaOK, 0, malformedSchemaMetadataError(dbPath)
	}

	var count int
	var minVersion, maxVersion sql.NullInt64
	if err := db.QueryRowContext(ctx, schema.ReadVersionMetadataQuery).Scan(&count, &minVersion, &maxVersion); err != nil {
		return SchemaOK, 0, fmt.Errorf("%w: %s has malformed schema metadata: %w",
			ErrSchemaIncompatible, describePath(dbPath), err)
	}
	if count != 1 || !minVersion.Valid || !maxVersion.Valid || minVersion.Int64 != maxVersion.Int64 {
		return SchemaOK, 0, malformedSchemaMetadataError(dbPath)
	}
	if maxVersion.Int64 >= schema.Baseline && maxVersion.Int64 <= int64(target) {
		return SchemaOK, int(maxVersion.Int64), nil
	}
	remedy := "use the original executable to view this database, or pass a different --db path"
	if maxVersion.Int64 > int64(target) {
		remedy = "upgrade the viewer to open this database, or pass a different --db path"
	}
	logger.Error("database was written by a different schema version",
		zap.String("database", describePath(dbPath)),
		zap.Int64("file_version", maxVersion.Int64),
		zap.Int("expected_version", target),
		zap.String("remedy", remedy))
	return SchemaMismatch, 0, fmt.Errorf("%w: %s was written by schema version %d, "+
		"this build supports %d through %d -- %s",
		ErrSchemaIncompatible, describePath(dbPath), maxVersion.Int64, schema.Baseline, target, remedy)
}

func inspectUnstampedDatabase(ctx context.Context, db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, int, error) {
	var hasTelemetryTables int
	if err := db.QueryRowContext(ctx, schema.TelemetryTableExistsQuery).Scan(&hasTelemetryTables); err != nil {
		return SchemaOK, 0, fmt.Errorf("%w while probing for existing tables: %w", ErrStoreInitFailed, err)
	}
	if hasTelemetryTables != 0 {
		logger.Error("database holds data but carries no schema version, so it predates "+
			"versioning and its shape cannot be confirmed",
			zap.String("database", describePath(dbPath)),
			zap.Int("expected_version", schema.Version),
			zap.String("remedy", "use the original executable or pass a different --db path"))
		return SchemaPreVersioning, 0, fmt.Errorf("%w: %s holds telemetry tables but carries no schema "+
			"version -- use the original executable or pass a different --db path",
			ErrSchemaIncompatible, describePath(dbPath))
	}
	return SchemaOK, 0, nil
}

func malformedSchemaMetadataError(dbPath string) error {
	return fmt.Errorf("%w: %s has malformed schema metadata -- restore a known-good backup or pass a different --db path",
		ErrSchemaIncompatible, describePath(dbPath))
}

func describePath(dbPath string) string {
	if dbPath == "" {
		return "(in-memory)"
	}
	return dbPath
}
