package duckdbextension

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	settingsFolder = "settings"
	activeFile     = "active-database"
)

// Resolve storage at startup, not when printing help or creating a factory.
func databasePath(configured string) (string, error) {
	dir, dirErr := appDataDirectory()
	if configured != "" {
		path, err := filepath.Abs(configured)
		if err != nil {
			return "", err
		}
		if configured == ":memory:" || strings.Contains(path, "?") {
			return "", fmt.Errorf("db must be a database file path, not an in-memory database or connection options")
		}
		if dirErr == nil {
			if err := checkDatabaseLocation(dir, path); err != nil {
				return "", err
			}
		}
		return path, nil
	}

	if dirErr != nil {
		return "", dirErr
	}
	name, err := managedSelection(dir)
	if err != nil {
		return "", err
	}
	if name == "" {
		path := filepath.Join(dir, "current.duckdb")
		return path, checkManagedFile(path, true)
	}
	path := filepath.Join(dir, name)
	return path, checkManagedFile(path, false)
}

func checkManagedFile(path string, allowNew bool) error {
	info, err := os.Lstat(path)
	if allowNew && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("database %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database is not a regular file: %s", path)
	}
	return nil
}

func sameLocation(a, b string) bool {
	if a == b {
		return true
	}
	x, err := os.Stat(a)
	if err != nil {
		return false
	}
	y, err := os.Stat(b)
	return err == nil && os.SameFile(x, y)
}

func checkDatabaseLocation(dir, path string) error {
	settings := filepath.Join(dir, settingsFolder)
	if sameLocation(dir, path) {
		return fmt.Errorf("expected a database file inside %s", dir)
	}
	if sameLocation(dir, filepath.Dir(path)) {
		if _, err := managedSelection(dir); err != nil {
			return err
		}
		if err := checkManagedFile(path, true); err != nil {
			return err
		}
	}
	// Compare directory identity too: an alternate spelling or directory
	// alias must not turn a settings file into a database destination.
	for p := path; ; p = filepath.Dir(p) {
		if sameLocation(p, settings) {
			return fmt.Errorf("the viewer settings directory is reserved: %s", settings)
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	if sameLocation(path, filepath.Join(settings, activeFile)) {
		return fmt.Errorf("the database selection file is reserved")
	}
	return nil
}

func managedSelection(dir string) (string, error) {
	if err := ensurePrivateDirectory(dir); err != nil {
		return "", err
	}
	settings := filepath.Join(dir, settingsFolder)
	if err := ensurePrivateDirectory(settings); err != nil {
		return "", err
	}
	return readSelection(settings)
}

// A selection contains a single basename. Inspect the file before reading
// it so an unrelated file (including an existing DuckDB) is never replaced.
func readSelection(settings string) (string, error) {
	path := filepath.Join(settings, activeFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	const maxBytes = 4096 // more than a filename component on the supported platforms
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", fmt.Errorf("invalid database selection file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxBytes || !isSessionFilename(string(data)) {
		return "", fmt.Errorf("invalid database selection file: %s", path)
	}
	return string(data), nil
}

func appDataDirectory() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		root, err := os.UserConfigDir() // ~/Library/Application Support
		if err != nil {
			return "", err
		}
		dir = filepath.Join(root, "com.ctrlspice.otel-desktop-viewer")
	case "windows":
		root, err := os.UserCacheDir() // LocalAppData rather than roaming AppData
		if err != nil {
			return "", err
		}
		dir = filepath.Join(root, "OTel Desktop Viewer")
	default:
		root := os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(root) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(root, "otel-desktop-viewer")
	}
	if !filepath.IsAbs(dir) || strings.Contains(dir, "?") {
		return "", fmt.Errorf("invalid application data directory %q", dir)
	}
	return filepath.Clean(dir), nil
}

func ensurePrivateDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	// The database journal inherits its own file mode. Protect the whole
	// directory on Unix; Windows files inherit the user's LocalAppData ACL.
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("expected a private app data directory at %s", dir)
	}
	return checkDirectoryOwner(dir, info)
}

func isSessionFilename(name string) bool {
	return filepath.IsLocal(name) && !strings.ContainsAny(name, `/\?`) &&
		strings.EqualFold(filepath.Ext(name), ".duckdb") && !strings.ContainsRune(name, 0)
}

// External --db runs (including isolated fixtures) do not change the user's
// selection. Publish an app-data selection only after its database opens.
func rememberDatabase(path string) error {
	dir, err := appDataDirectory()
	if err != nil || !sameLocation(filepath.Dir(path), dir) || !isSessionFilename(filepath.Base(path)) {
		return nil
	}
	settings := filepath.Join(dir, settingsFolder)
	previous, err := managedSelection(dir)
	if err != nil || previous == filepath.Base(path) {
		return err
	}
	tmp, err := os.CreateTemp(settings, ".active-database-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.WriteString(filepath.Base(path))
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(settings, activeFile))
}
