package store

import (
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"go.uber.org/zap"
)

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

// checkSchemaVersion inspects the version stamp and decides whether this build
// can be expected to read the file.
//
// Four outcomes:
//
//	stamp absent, no data   -> brand new; stamp it and proceed
//	stamp absent, has data  -> written before versioning; warn
//	stamp present, matches  -> proceed
//	stamp present, differs  -> warn
//
// A stamp-less file with data is treated as suspect rather than stamped,
// because stamping it would assert a compatibility nobody has checked and
// destroy the only evidence that it predates versioning.
func checkSchemaVersion(db *sql.DB, dbPath string, logger *zap.Logger) (SchemaCompatibility, error) {
	if _, err := db.Exec(schema.VersionTableQuery); err != nil {
		return SchemaOK, fmt.Errorf("%w while creating schema_meta: %w", ErrStoreInitFailed, err)
	}

	// NULL when the table is empty, so scan through a nullable.
	var stamped sql.NullInt64
	if err := db.QueryRow(schema.ReadVersionQuery).Scan(&stamped); err != nil {
		return SchemaOK, fmt.Errorf("%w while reading schema version: %w", ErrStoreInitFailed, err)
	}

	if stamped.Valid {
		if stamped.Int64 == schema.Version {
			return SchemaOK, nil
		}
		logger.Warn("database was written by a different schema version; "+
			"the viewer may fail to read or write it",
			zap.String("database", describePath(dbPath)),
			zap.Int64("file_version", stamped.Int64),
			zap.Int("expected_version", schema.Version),
			zap.String("remedy", "delete it or pass a different --db path"))
		return SchemaMismatch, nil
	}

	hasData, err := hasExistingData(db)
	if err != nil {
		return SchemaOK, err
	}
	if hasData {
		logger.Warn("database holds data but carries no schema version, so it predates "+
			"versioning and its shape cannot be confirmed",
			zap.String("database", describePath(dbPath)),
			zap.Int("expected_version", schema.Version),
			zap.String("remedy", "delete it or pass a different --db path"))
		return SchemaPreVersioning, nil
	}

	if _, err := db.Exec(schema.StampVersionQuery, schema.Version); err != nil {
		return SchemaOK, fmt.Errorf("%w while stamping schema version: %w", ErrStoreInitFailed, err)
	}
	return SchemaOK, nil
}

// hasExistingData reports whether the file already holds telemetry. Two queries
// rather than one: DuckDB binds a whole statement before running it, so a
// subquery naming `spans` fails to bind on a database that has never had it.
func hasExistingData(db *sql.DB) (bool, error) {
	var tables int
	if err := db.QueryRow(schema.SpansTableExistsQuery).Scan(&tables); err != nil {
		return false, fmt.Errorf("%w while probing for existing tables: %w", ErrStoreInitFailed, err)
	}
	if tables == 0 {
		return false, nil
	}

	var spans int64
	if err := db.QueryRow(schema.SpanCountQuery).Scan(&spans); err != nil {
		return false, fmt.Errorf("%w while probing for existing data: %w", ErrStoreInitFailed, err)
	}
	return spans > 0, nil
}

func describePath(dbPath string) string {
	if dbPath == "" {
		return "(in-memory)"
	}
	return dbPath
}
