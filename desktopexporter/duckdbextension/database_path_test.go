package duckdbextension

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.uber.org/zap"
)

func TestDefaultDatabaseResumesInUserAppData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative-path-ignored")
	t.Setenv("LOCALAPPDATA", home)
	want := filepath.Join(home, ".local", "share", "otel-desktop-viewer", "current.duckdb")
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join(home, "Library", "Application Support", "com.ctrlspice.otel-desktop-viewer", "current.duckdb")
	case "windows":
		want = filepath.Join(home, "OTel Desktop Viewer", "current.duckdb")
	}
	path, create, err := databasePath("")
	require.NoError(t, err)
	require.True(t, create)
	require.Equal(t, want, path)
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0600))
	again, create, err := databasePath("")
	require.NoError(t, err)
	require.False(t, create)
	require.Equal(t, path, again)
	other := filepath.Join(filepath.Dir(path), "another trace.duckdb")
	require.NoError(t, os.WriteFile(other, nil, 0600))
	require.NoError(t, rememberDatabase(other))
	require.NoError(t, rememberDatabase(filepath.Join(t.TempDir(), "fixture.duckdb")))
	again, _, err = databasePath("")
	require.NoError(t, err)
	require.Equal(t, other, again)
	require.NoError(t, os.Remove(other))
	_, _, err = databasePath("")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, _, err = databasePath(":memory:")
	require.Error(t, err)
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(filepath.Dir(path), 0755))
		_, _, err = databasePath("")
		require.ErrorContains(t, err, "private app data directory")
	}
}

func TestRestartUsesLastActiveDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	ctx := t.Context()
	dir, err := appDataDirectory()
	require.NoError(t, err)
	run := func(path string, use func(*sql.DB) error) {
		t.Helper()
		e := &DuckDBExtension{cfg: &Config{Db: path, Endpoint: "127.0.0.1:0"}, logger: zap.NewNop()}
		require.NoError(t, e.Start(ctx, componenttest.NewNopHost()))
		defer func() { require.NoError(t, e.Shutdown(ctx)) }()
		require.NoError(t, e.Ready())
		require.NoError(t, e.Store().WithDBWrite(use))
	}
	mark := func(label string) func(*sql.DB) error {
		return func(db *sql.DB) error {
			_, err := db.Exec(`create table session_marker as select ? as label`, label)
			return err
		}
	}
	run("", mark("first"))
	// The former metadata filename is available for ordinary databases.
	oldName := filepath.Join(dir, "last-database")
	run(oldName, mark("keep this database"))
	run(filepath.Join(dir, "checkout.duckdb"), mark("checkout"))
	run(filepath.Join(t.TempDir(), "external.duckdb"), mark("external"))
	run("", func(db *sql.DB) error {
		var label string
		err := db.QueryRow(`select label from session_marker`).Scan(&label)
		require.Equal(t, "checkout", label)
		return err
	})
	run(oldName, func(db *sql.DB) error {
		var label string
		err := db.QueryRow(`select label from session_marker`).Scan(&label)
		require.Equal(t, "keep this database", label)
		return err
	})
	active := filepath.Join(dir, "settings", "active-database")
	bytes, err := os.ReadFile(active)
	require.NoError(t, err)
	require.Equal(t, "checkout.duckdb", string(bytes))
	_, _, err = databasePath(active)
	require.Error(t, err)
	_, _, err = databasePath(filepath.Join(dir, "settings", "other.duckdb"))
	require.Error(t, err)

	// Preserve an existing database even if someone placed it at the new
	// metadata location outside the viewer. Neither startup path may consume it.
	require.NoError(t, os.Remove(active))
	require.NoError(t, os.Rename(oldName, active))
	original, err := os.ReadFile(active)
	require.NoError(t, err)
	_, _, err = databasePath("")
	require.Error(t, err)
	require.Error(t, rememberDatabase(filepath.Join(dir, "checkout.duckdb")))
	remaining, err := os.ReadFile(active)
	require.NoError(t, err)
	require.Equal(t, original, remaining)
}

func TestAutomaticStartupRejectsUnrelatedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	path, _, err := databasePath("")
	require.NoError(t, err)
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create macro attr_id(k, v) as 'original'`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	e := &DuckDBExtension{cfg: &Config{Endpoint: "127.0.0.1:0"}, logger: zap.NewNop()}
	err = e.Start(t.Context(), componenttest.NewNopHost())
	require.ErrorContains(t, err, path)
	require.ErrorContains(t, err, "not a recognized viewer database")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoFileExists(t, filepath.Join(filepath.Dir(path), settingsFolder, activeFile))
}

func TestManagedDatabaseLocationBeforeOpen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	dir, err := appDataDirectory()
	require.NoError(t, err)
	other := filepath.Join(dir, "checkout.duckdb")
	path, _, err := databasePath(other)
	require.NoError(t, err)
	require.Equal(t, other, path)
	require.NoFileExists(t, other)
	require.DirExists(t, filepath.Join(dir, settingsFolder))
	if runtime.GOOS == "windows" {
		return // Unix permissions and symlinks below.
	}
	external := t.TempDir()
	require.NoError(t, os.Chmod(external, 0755))
	ext := filepath.Join(external, "external.duckdb")
	require.NoError(t, os.WriteFile(ext, []byte("unchanged"), 0600))
	require.NoError(t, os.Symlink(ext, other))
	require.NoError(t, os.Symlink(ext, filepath.Join(dir, "current.duckdb")))
	for _, arg := range []string{"", other} {
		_, _, err := databasePath(arg)
		require.ErrorContains(t, err, "not a regular file")
	}
	require.NoError(t, os.Chmod(dir, 0755))
	for _, arg := range []string{"", filepath.Join(dir, "third.duckdb")} {
		_, _, err := databasePath(arg)
		require.ErrorContains(t, err, "private app data directory")
	}
	path, _, err = databasePath(ext)
	require.NoError(t, err)
	require.Equal(t, ext, path)
	info, err := os.Stat(external)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestPercentPathsReopenExactDatabase(t *testing.T) {
	home := filepath.Join(t.TempDir(), "100% complete")
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	dir, err := appDataDirectory()
	require.NoError(t, err)
	ctx := t.Context()
	use := func(arg, want, label string, write bool) {
		t.Helper()
		e := &DuckDBExtension{cfg: &Config{Db: arg, Endpoint: "127.0.0.1:0"}, logger: zap.NewNop()}
		require.NoError(t, e.Start(ctx, componenttest.NewNopHost()))
		defer func() { require.NoError(t, e.Shutdown(ctx)) }()
		require.NoError(t, e.Ready())
		require.NoError(t, e.Store().WithDBWrite(func(db *sql.DB) error {
			file, err := os.Stat(want)
			require.NoError(t, err)
			if write {
				_, err = db.Exec(`create table marker as select ? as label`, label)
				require.NoError(t, err)
			}
			// Hold both connections so the second one cannot reuse the first.
			for range 2 {
				conn, err := db.Conn(ctx)
				require.NoError(t, err)
				defer conn.Close()
				var opened, stored string
				err = conn.QueryRowContext(ctx, `select path, label from duckdb_databases(), marker where database_name = current_database()`).Scan(&opened, &stored)
				require.NoError(t, err)
				actual, err := os.Stat(opened)
				require.NoError(t, err)
				require.True(t, os.SameFile(file, actual))
				require.Equal(t, label, stored)
			}
			return nil
		}))
	}
	first := filepath.Join(dir, "current.duckdb")
	other := filepath.Join(dir, "O'Brien 50%.duckdb")
	use("", first, "first", true)
	use("", first, "first", false)
	use(other, other, "second", true)
	use("", other, "second", false)
}
