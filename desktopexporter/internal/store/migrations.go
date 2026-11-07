package store

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"go.uber.org/zap"
)

// initializeSchema runs under schemaInitializationMu before the store is exposed
// to readers or ingestion. Unsupported files are rejected before any DDL.
func initializeSchema(ctx context.Context, db *sql.DB, dbPath string, logger *zap.Logger, target int, migrations []schema.Migration) (SchemaCompatibility, error) {
	// Validate the shipped chain even when creating a fresh database.
	if _, err := schema.Plan(0, target, migrations); err != nil {
		return SchemaOK, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}
	compat, current, err := inspectSchemaVersion(ctx, db, dbPath, logger, target)
	if err != nil {
		return compat, err
	}
	pending, err := schema.Plan(current, target, migrations)
	if err != nil {
		return compat, fmt.Errorf("%w: %w", ErrStoreInitFailed, err)
	}
	// Native functions are connection registrations, not persisted schema objects.
	if err := queries.RegisterFunctions(ctx, db); err != nil {
		return compat, fmt.Errorf("%w while registering value functions: %w", ErrStoreInitFailed, err)
	}
	ddl := append(queries.Types(), queries.Tables()...)
	ddl = append(ddl, queries.Indexes()...)
	ddl = append(ddl, queries.Macros()...)
	if err := applySchema(ctx, db, current, target, pending, ddl); err != nil {
		return compat, fmt.Errorf("%w for %s: %w", ErrStoreInitFailed, describePath(dbPath), err)
	}
	if len(pending) != 0 {
		logger.Info("database schema upgraded", zap.String("database", describePath(dbPath)),
			zap.Int("from_version", current), zap.Int("to_version", target))
	}
	return compat, nil
}

// applySchema commits all upgrades, current DDL, and the final stamp together.
// Fresh stores follow the same transaction boundary without replaying history.
func applySchema(ctx context.Context, db *sql.DB, current, target int, pending []schema.Migration, ddl []queries.Statement) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema transaction: %w", err)
	}
	defer tx.Rollback()
	for _, migration := range pending {
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("migration %s (schema %d to %d): %w", migration.Name, migration.Version-1, migration.Version, err)
		}
	}
	for _, stmt := range ddl {
		// Type files are named after their types. Probe instead of swallowing a
		// duplicate CREATE TYPE error, which would abort the transaction.
		if strings.HasPrefix(stmt.Name, "types/") {
			var exists bool
			name := strings.TrimSuffix(path.Base(stmt.Name), ".sql")
			if err := tx.QueryRowContext(ctx, `select exists(select 1 from duckdb_types()
				where database_name = current_database() and schema_name = current_schema() and type_name = ?)`, name).Scan(&exists); err != nil {
				return fmt.Errorf("inspect type %s: %w", stmt.Name, err)
			}
			if exists {
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, stmt.SQL); err != nil {
			return fmt.Errorf("apply DDL %s: %w", stmt.Name, err)
		}
	}
	if current == 0 {
		if _, err := tx.ExecContext(ctx, schema.VersionTableQuery); err != nil {
			return fmt.Errorf("create schema metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, schema.StampVersionQuery, target); err != nil {
			return fmt.Errorf("stamp schema version: %w", err)
		}
	} else if current != target {
		if _, err := tx.ExecContext(ctx, `update schema_meta set version = ?`, target); err != nil {
			return fmt.Errorf("update schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema transaction: %w", err)
	}
	return nil
}
