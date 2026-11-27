package duckdbextension

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/pdata/plog"
)

func startManagedExtension(t *testing.T, managedDir string) *DuckDBExtension {
	t.Helper()
	ctx := context.Background()
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.Managed = true
	cfg.ManagedDir = managedDir
	cfg.DbMaxSize = "0"

	comp, err := factory.Create(ctx, extensiontest.NewNopSettings(Type), cfg)
	require.NoError(t, err)
	ext := comp.(*DuckDBExtension)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	return ext
}

func ingestTestLog(t *testing.T, s *store.Store, body string) {
	t.Helper()
	batch := plog.NewLogs()
	rec := batch.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr(body)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(context.Background(), conn, batch)
	}))
}

func readTestLogBodies(t *testing.T, s *store.Store) []string {
	t.Helper()
	var bodies []string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(context.Background(), `select body::varchar from logs order by body::varchar`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				return err
			}
			bodies = append(bodies, b)
		}
		return rows.Err()
	}))
	return bodies
}

func TestManagedSessionsPersistAcrossTwoLaunchesAndSwitches(t *testing.T) {
	ctx := context.Background()
	managedDir := filepath.Join(t.TempDir(), "app-data")

	// Launch 1: creates current.duckdb automatically, ingests telemetry,
	// creates a second named session, ingests separate telemetry, then stops.
	ext1 := startManagedExtension(t, managedDir)
	list, err := ext1.Databases().ListDatabases(ctx)
	require.NoError(t, err)
	assert.True(t, list.Managed)
	assert.Equal(t, "current.duckdb", list.Active)
	require.Len(t, list.Databases, 1)
	assert.True(t, list.Databases[0].Active)

	ingestTestLog(t, ext1.Store(), "first-launch-current")

	// Create a new session without needing to type ".duckdb".
	list, err = ext1.Databases().NewDatabase(ctx, "checkout-incident")
	require.NoError(t, err)
	assert.Equal(t, "checkout-incident.duckdb", list.Active)
	require.Len(t, list.Databases, 2)
	assert.Empty(t, readTestLogBodies(t, ext1.Store()))

	ingestTestLog(t, ext1.Store(), "incident-evidence")
	require.NoError(t, ext1.Shutdown(ctx))

	// Launch 2: automatically reopens the last active session ("checkout-incident.duckdb")
	// and preserves both sessions cleanly.
	ext2 := startManagedExtension(t, managedDir)
	defer func() { require.NoError(t, ext2.Shutdown(ctx)) }()

	list, err = ext2.Databases().ListDatabases(ctx)
	require.NoError(t, err)
	assert.Equal(t, "checkout-incident.duckdb", list.Active)
	assert.Equal(t, []string{"incident-evidence"}, readTestLogBodies(t, ext2.Store()))

	// Switch back to current.duckdb and verify its data survived.
	list, err = ext2.Databases().OpenDatabase(ctx, "current.duckdb")
	require.NoError(t, err)
	assert.Equal(t, "current.duckdb", list.Active)
	assert.Equal(t, []string{"first-launch-current"}, readTestLogBodies(t, ext2.Store()))
}

func TestManagedSessionsRejectInvalidNamesAndMissingSelection(t *testing.T) {
	ctx := context.Background()
	managedDir := filepath.Join(t.TempDir(), "app-data")

	ext := startManagedExtension(t, managedDir)
	for _, bad := range []string{"", "   ", "../escape.duckdb", "sub/dir.duckdb", ".hidden.duckdb", "a?b=1.duckdb", ".duckdb"} {
		_, err := ext.Databases().NewDatabase(ctx, bad)
		require.ErrorIs(t, err, server.ErrInvalidDatabaseName, "expected invalid name for %q", bad)
	}
	_, err := ext.Databases().OpenDatabase(ctx, "../escape.duckdb")
	require.ErrorIs(t, err, server.ErrInvalidDatabaseName)

	_, err = ext.Databases().NewDatabase(ctx, "current")
	require.ErrorIs(t, err, store.ErrDatabaseAlreadyExists)

	_, err = ext.Databases().OpenDatabase(ctx, "nonexistent.duckdb")
	require.ErrorIs(t, err, store.ErrDatabaseNotFound)
	require.NoError(t, ext.Shutdown(ctx))

	// If the remembered file disappears externally, refuse to silently create
	// an empty file so the user notices immediately.
	require.NoError(t, os.Remove(filepath.Join(managedDir, "current.duckdb")))

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.Managed = true
	cfg.ManagedDir = managedDir
	cfg.DbMaxSize = "0"
	comp, err := factory.Create(ctx, extensiontest.NewNopSettings(Type), cfg)
	require.NoError(t, err)
	err = comp.Start(ctx, componenttest.NewNopHost())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestExternalDBModeIsIndependentOfManagedSessions(t *testing.T) {
	ctx := context.Background()
	externalPath := filepath.Join(t.TempDir(), "external.duckdb")

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.Db = externalPath
	cfg.DbMaxSize = "0"

	comp, err := factory.Create(ctx, extensiontest.NewNopSettings(Type), cfg)
	require.NoError(t, err)
	ext := comp.(*DuckDBExtension)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	defer func() { require.NoError(t, ext.Shutdown(ctx)) }()

	list, err := ext.Databases().ListDatabases(ctx)
	require.NoError(t, err)
	assert.False(t, list.Managed)
	assert.Equal(t, "external.duckdb", list.Active)
	assert.Empty(t, list.Databases)

	_, err = ext.Databases().NewDatabase(ctx, "other")
	require.ErrorIs(t, err, store.ErrDatabaseSwitchUnsupported)
}

func TestConfigRejectsBothDbAndManaged(t *testing.T) {
	cfg := Config{Endpoint: "localhost:8000", Db: "/tmp/a.duckdb", Managed: true}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot both be configured")
}
