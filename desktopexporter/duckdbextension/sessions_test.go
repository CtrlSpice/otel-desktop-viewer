package duckdbextension

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	for _, bad := range []string{
		"",
		"   ",
		"../escape.duckdb",
		"sub/dir.duckdb",
		".hidden.duckdb",
		"a?b=1.duckdb",
		".duckdb",
		"cpu-100%",
		"cpu-100%.duckdb",
		"line\nbreak.duckdb",
		"tab\tname.duckdb",
		"cr\rname.duckdb",
		"del\x7fname.duckdb",
		"null\x00byte.duckdb",
		"colon:name.duckdb",
		"star*name.duckdb",
		`quote"name.duckdb`,
		"lt<name.duckdb",
		"gt>name.duckdb",
		"pipe|name.duckdb",
		"trailing-dot..duckdb",
		"trailing-space .duckdb",
		strings.Repeat("a", 194) + ".duckdb",
	} {
		_, err := ext.Databases().NewDatabase(ctx, bad)
		require.ErrorIs(t, err, server.ErrInvalidDatabaseName, "expected invalid name for %q", bad)
	}
	for _, bad := range []string{"../escape.duckdb", "cpu-100%.duckdb", "a\nb.duckdb", "a:b.duckdb", "foo..duckdb"} {
		_, err := ext.Databases().OpenDatabase(ctx, bad)
		require.ErrorIs(t, err, server.ErrInvalidDatabaseName, "expected invalid name on open for %q", bad)
	}

	_, err := ext.Databases().NewDatabase(ctx, "current")
	require.ErrorIs(t, err, store.ErrDatabaseAlreadyExists)

	// Case-insensitive collision must also return ErrDatabaseAlreadyExists.
	_, err = ext.Databases().NewDatabase(ctx, "CURRENT")
	require.ErrorIs(t, err, store.ErrDatabaseAlreadyExists)
	_, err = ext.Databases().NewDatabase(ctx, "Current.duckdb")
	require.ErrorIs(t, err, store.ErrDatabaseAlreadyExists)

	// Opening with different casing resolves to the canonical on-disk filename
	// and keeps Active synchronized across listDatabases and settings/active-database.
	list, err := ext.Databases().OpenDatabase(ctx, "CURRENT.duckdb")
	require.NoError(t, err)
	assert.Equal(t, "current.duckdb", list.Active)
	require.Len(t, list.Databases, 1)
	assert.True(t, list.Databases[0].Active)
	assert.Equal(t, "current.duckdb", list.Databases[0].Name)
	selectedBytes, err := os.ReadFile(filepath.Join(managedDir, settingsFolder, activeFile))
	require.NoError(t, err)
	assert.Equal(t, "current.duckdb", string(selectedBytes))

	_, err = ext.Databases().OpenDatabase(ctx, "nonexistent.duckdb")
	require.ErrorIs(t, err, store.ErrDatabaseNotFound)
	require.NoError(t, ext.Shutdown(ctx))

	// If the remembered file disappears externally, refuse to silently create
	// an empty file so the user notices immediately.
	require.NoError(t, os.Remove(filepath.Join(managedDir, "current.duckdb")))

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.ManagedDir = managedDir
	cfg.DbMaxSize = "0"
	comp, err := factory.Create(ctx, extensiontest.NewNopSettings(Type), cfg)
	require.NoError(t, err)
	err = comp.Start(ctx, componenttest.NewNopHost())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestManagedSessionsCorruptSelectionRemediation(t *testing.T) {
	ctx := context.Background()
	managedDir := filepath.Join(t.TempDir(), "app-data")
	settingsDir, err := ensureManagedDirs(managedDir)
	require.NoError(t, err)

	// Empty (e.g. post-crash 0-byte) active-database gives an actionable hint.
	selectionPath := filepath.Join(settingsDir, activeFile)
	require.NoError(t, os.WriteFile(selectionPath, nil, 0600))

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"
	cfg.ManagedDir = managedDir
	cfg.DbMaxSize = "0"
	comp, err := factory.Create(ctx, extensiontest.NewNopSettings(Type), cfg)
	require.NoError(t, err)
	err = comp.Start(ctx, componenttest.NewNopHost())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty or corrupted")
	assert.Contains(t, err.Error(), selectionPath)
	assert.Contains(t, err.Error(), defaultSessionName)
}

func TestManagedSessionsRelativeManagedDirAndSubsecondSort(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(parent))
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	ext := startManagedExtension(t, "relative-sessions")
	defer func() { require.NoError(t, ext.Shutdown(ctx)) }()

	// Switching must work even when ManagedDir was configured as a relative path.
	_, err = ext.Databases().NewDatabase(ctx, "alpha")
	require.NoError(t, err)
	_, err = ext.Databases().NewDatabase(ctx, "zeta")
	require.NoError(t, err)
	_, err = ext.Databases().OpenDatabase(ctx, "current.duckdb")
	require.NoError(t, err)

	// Give alpha and zeta timestamps within the same second, with zeta strictly newer.
	base := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	alphaPath := filepath.Join(parent, "relative-sessions", "alpha.duckdb")
	zetaPath := filepath.Join(parent, "relative-sessions", "zeta.duckdb")
	require.NoError(t, os.Chtimes(alphaPath, base.Add(100*time.Millisecond), base.Add(100*time.Millisecond)))
	require.NoError(t, os.Chtimes(zetaPath, base.Add(900*time.Millisecond), base.Add(900*time.Millisecond)))

	list, err := ext.Databases().ListDatabases(ctx)
	require.NoError(t, err)
	require.Len(t, list.Databases, 3)
	assert.Equal(t, "current.duckdb", list.Databases[0].Name, "active database sorts first")
	// On filesystems with sub-second mtime resolution, zeta precedes alpha despite
	// having the same RFC3339 whole-second string.
	alphaInfo, err := os.Stat(alphaPath)
	require.NoError(t, err)
	zetaInfo, err := os.Stat(zetaPath)
	require.NoError(t, err)
	if zetaInfo.ModTime().After(alphaInfo.ModTime()) {
		assert.Equal(t, "zeta.duckdb", list.Databases[1].Name)
		assert.Equal(t, "alpha.duckdb", list.Databases[2].Name)
	}
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

func TestConfigRejectsBothDbAndManagedDir(t *testing.T) {
	cfg := Config{Endpoint: "localhost:8000", Db: "/tmp/a.duckdb", ManagedDir: "/tmp/managed"}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot both be configured")
}
