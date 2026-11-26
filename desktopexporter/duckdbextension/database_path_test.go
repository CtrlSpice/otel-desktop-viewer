package duckdbextension

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/plog"
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

func TestSelectionCanChangeWhilePreviousFileIsOpen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	dir, err := appDataDirectory()
	require.NoError(t, err)
	require.NoError(t, rememberDatabase(filepath.Join(dir, "first.duckdb")))
	settings := filepath.Join(dir, settingsFolder)
	reader, err := openSelection(settings)
	require.NoError(t, err)
	defer reader.Close()
	require.NoError(t, rememberDatabase(filepath.Join(dir, "second.duckdb")))
	old, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "first.duckdb", string(old))
	next, err := readSelection(settings)
	require.NoError(t, err)
	require.Equal(t, "second.duckdb", next)
}

func TestManagedOpenAndNewSurviveCollectorRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	ctx := t.Context()
	start := func() *DuckDBExtension {
		t.Helper()
		e := &DuckDBExtension{cfg: &Config{Endpoint: "127.0.0.1:0"}, logger: zap.NewNop()}
		require.NoError(t, e.Start(ctx, componenttest.NewNopHost()))
		require.NoError(t, e.Ready())
		return e
	}
	e := start()
	t.Cleanup(func() { require.NoError(t, e.Shutdown(ctx)) })
	manager := managedDatabases{active: e.Store()}
	write := func(label string) {
		t.Helper()
		require.NoError(t, e.Store().WithDBWrite(func(db *sql.DB) error {
			_, err := db.Exec(`create table session_marker as select ? as label`, label)
			return err
		}))
	}
	read := func() string {
		t.Helper()
		var label string
		require.NoError(t, e.Store().WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select label from session_marker`).Scan(&label)
		}))
		return label
	}
	write("original")
	require.Error(t, manager.open(ctx, "../outside.duckdb"))
	require.Error(t, manager.new(ctx, "../outside.duckdb"))
	require.Error(t, manager.open(ctx, "missing.duckdb"))
	dir, err := appDataDirectory()
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "missing.duckdb"))
	require.NoError(t, manager.new(ctx, "checkout.duckdb"))
	write("checkout")
	require.ErrorIs(t, manager.new(ctx, "current.duckdb"), os.ErrExist)
	require.Equal(t, "checkout", read())
	require.NoError(t, manager.open(ctx, "current.duckdb"))
	require.Equal(t, "original", read())
	require.NoError(t, manager.open(ctx, "checkout.duckdb"))
	require.NoError(t, e.Shutdown(ctx))
	e = start()
	require.Equal(t, "checkout", read())
}

func TestCrashDuringManagedDatabaseSwitch(t *testing.T) {
	const env = "VIEWER_TEST_SWITCH_EXIT"
	ctx := t.Context()
	open := func(path string, create bool) *store.Store {
		t.Helper()
		s, err := store.OpenViewerDatabase(ctx, path, create, nil)
		require.NoError(t, err)
		return s
	}
	if phase := os.Getenv(env); phase != "" {
		first, create, err := databasePath("")
		require.NoError(t, err)
		active := open(first, create)
		require.NoError(t, rememberDatabase(first))
		writeBatch := func(label string) {
			t.Helper()
			batch := plog.NewLogs()
			batch.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetSeverityText(label)
			require.NoError(t, active.WithConn(func(c driver.Conn) error { return logs.Ingest(ctx, c, batch) }))
		}
		writeBatch("before")
		second := filepath.Join(filepath.Dir(first), "checkout.duckdb")
		require.NoError(t, active.CreateDatabase(ctx, second, func() error {
			if phase == "before-remember" {
				os.Exit(0)
			}
			require.NoError(t, rememberDatabase(second))
			if phase == "after-remember" {
				os.Exit(0)
			}
			return nil
		}))
		writeBatch("after")
		os.Exit(0)
	}
	for _, phase := range []string{"before-remember", "after-remember", "after-ingest"} {
		t.Run(phase, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", home)
			t.Setenv("LOCALAPPDATA", home)
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashDuringManagedDatabaseSwitch$")
			cmd.Env = append(os.Environ(), env+"="+phase)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			selected, create, err := databasePath("")
			require.NoError(t, err)
			require.False(t, create)
			want := "checkout.duckdb"
			if phase == "before-remember" {
				want = "current.duckdb"
			}
			require.Equal(t, want, filepath.Base(selected))
			for _, name := range []string{"current.duckdb", "checkout.duckdb"} {
				s := open(filepath.Join(filepath.Dir(selected), name), false)
				var batches string
				require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
					return db.QueryRow(`select coalesce(string_agg(severity_text, ',' order by severity_text), '') from logs`).Scan(&batches)
				}))
				require.NoError(t, s.Close())
				switch {
				case name == "current.duckdb":
					require.Equal(t, "before", batches)
				case phase == "after-ingest":
					require.Equal(t, "after", batches)
				default:
					require.Empty(t, batches)
				}
			}
		})
	}
}

func TestCollectorRecoversCommittedLogsThroughAnotherHardLink(t *testing.T) {
	ctx := t.Context()
	start := func(path string) *DuckDBExtension {
		t.Helper()
		e := &DuckDBExtension{cfg: &Config{Endpoint: "127.0.0.1:0", Db: path}, logger: zap.NewNop()}
		require.NoError(t, e.Start(ctx, componenttest.NewNopHost()))
		require.NoError(t, e.Ready())
		return e
	}
	const env = "VIEWER_TEST_ALIAS_PATH"
	if path := os.Getenv(env); path != "" {
		e := start(path)
		batch := plog.NewLogs()
		batch.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetSeverityText(filepath.Base(path))
		require.NoError(t, e.Store().WithConn(func(c driver.Conn) error { return logs.Ingest(ctx, c, batch) }))
		os.Exit(0) // Do not run DuckDB shutdown or checkpoint the committed log.
	}
	home := t.TempDir()
	for _, key := range []string{"HOME", "XDG_DATA_HOME", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	a := filepath.Join(t.TempDir(), "first.duckdb")
	b := filepath.Join(t.TempDir(), "second.duckdb")
	e := start(a)
	require.NoError(t, e.Shutdown(ctx))
	require.NoError(t, os.Link(a, b))
	for _, path := range []string{a, b} {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCollectorRecoversCommittedLogsThroughAnotherHardLink$")
		cmd.Env = append(os.Environ(), env+"="+path)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		wal, err := os.Stat(path + ".wal")
		require.NoError(t, err)
		require.Positive(t, wal.Size())
	}
	for _, path := range []string{a, b} {
		e = start(path)
		var labels, catalog string
		require.NoError(t, e.Store().WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select current_database(), string_agg(severity_text, ',' order by severity_text) from logs`).Scan(&catalog, &labels)
		}))
		require.Equal(t, filepath.Base(path[:len(path)-len(".duckdb")]), catalog)
		require.Equal(t, "first.duckdb,second.duckdb", labels)
		require.NoError(t, e.Shutdown(ctx))
	}
}

func TestPercentPathsReopenExactDatabase(t *testing.T) {
	home := filepath.Join(t.TempDir(), "100% complete")
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	dir, err := appDataDirectory()
	require.NoError(t, err)
	ctx := t.Context()
	use := func(arg, label string, write bool) {
		t.Helper()
		e := &DuckDBExtension{cfg: &Config{Db: arg, Endpoint: "127.0.0.1:0"}, logger: zap.NewNop()}
		require.NoError(t, e.Start(ctx, componenttest.NewNopHost()))
		defer func() { require.NoError(t, e.Shutdown(ctx)) }()
		require.NoError(t, e.Ready())
		require.NoError(t, e.Store().WithDBWrite(func(db *sql.DB) error {
			if write {
				_, err := db.Exec(`create table marker as select ? as label`, label)
				return err
			}
			var stored string
			require.NoError(t, db.QueryRowContext(ctx, `select label from marker`).Scan(&stored))
			require.Equal(t, label, stored)
			return nil
		}))
	}
	other := filepath.Join(dir, "O'Brien 50%.duckdb")
	use("", "first", true)
	require.FileExists(t, filepath.Join(dir, "current.duckdb"))
	use("", "first", false)
	use(other, "second", true)
	require.FileExists(t, other)
	use("", "second", false)
}
