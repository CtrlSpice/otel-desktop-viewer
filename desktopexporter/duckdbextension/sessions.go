package duckdbextension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

const (
	defaultSessionName = "current.duckdb"
	settingsFolder     = "settings"
	activeFile         = "active-database"
	maxSessionNameLen  = 200
)

// DefaultAppDataDir returns the OS-specific private directory where the viewer
// stores managed session databases when launched without --db.
func DefaultAppDataDir() (string, error) {
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
	if !filepath.IsAbs(dir) || strings.ContainsAny(dir, "?%") {
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

func ensureManagedDirs(dir string) (string, error) {
	if err := ensurePrivateDirectory(dir); err != nil {
		return "", err
	}
	settings := filepath.Join(dir, settingsFolder)
	if err := ensurePrivateDirectory(settings); err != nil {
		return "", err
	}
	return settings, nil
}

func isSessionFilename(name string) bool {
	if name == "" || len(name) > maxSessionNameLen || strings.HasPrefix(name, ".") || strings.TrimSpace(name) != name {
		return false
	}
	if !filepath.IsLocal(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
		switch r {
		case '/', '\\', '?', '%', ':', '*', '"', '<', '>', '|':
			return false
		}
	}
	ext := filepath.Ext(name)
	if !strings.EqualFold(ext, ".duckdb") {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	if strings.TrimSpace(stem) == "" || strings.HasSuffix(stem, ".") || strings.HasSuffix(stem, " ") {
		return false
	}
	return true
}

// findSessionEntry resolves the canonical on-disk filename for name in dir,
// preferring an exact match and falling back to a case-insensitive match so
// APFS/NTFS casing differences never desynchronize active-session state.
func findSessionEntry(dir, name string) (canonical string, found bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, err
	}
	var foldMatch string
	for _, entry := range entries {
		entryName := entry.Name()
		if entryName == name {
			return entryName, true, nil
		}
		if foldMatch == "" && strings.EqualFold(entryName, name) {
			foldMatch = entryName
		}
	}
	if foldMatch != "" {
		return foldMatch, true, nil
	}
	return "", false, nil
}

// normalizeNewSessionName trims whitespace and appends .duckdb when omitted so
// browser users can enter plain names like "checkout-incident".
func normalizeNewSessionName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: session name cannot be empty", server.ErrInvalidDatabaseName)
	}
	if !strings.EqualFold(filepath.Ext(name), ".duckdb") {
		name += ".duckdb"
	}
	if !isSessionFilename(name) {
		return "", fmt.Errorf("%w: %q must be a plain .duckdb filename", server.ErrInvalidDatabaseName, raw)
	}
	return name, nil
}

func readSelection(settings string) (string, error) {
	path := filepath.Join(settings, activeFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	const maxBytes = 4096
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", fmt.Errorf("invalid database selection file %s (remove %s to resume with %s)", path, path, defaultSessionName)
	}
	f, err := os.OpenInRoot(settings, activeFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(data))
	if len(data) > maxBytes || !isSessionFilename(name) {
		return "", fmt.Errorf("database selection file %s is empty or corrupted (remove %s to resume with %s)", path, path, defaultSessionName)
	}
	return name, nil
}

func writeSelection(dir, name string) error {
	if !isSessionFilename(name) {
		return fmt.Errorf("%w: %q", server.ErrInvalidDatabaseName, name)
	}
	settings, err := ensureManagedDirs(dir)
	if err != nil {
		return err
	}
	previous, err := readSelection(settings)
	if err != nil {
		return err
	}
	if previous == name {
		return nil
	}
	tmp, err := os.CreateTemp(settings, ".active-database-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(name); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	root, err := os.OpenRoot(settings)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Rename(filepath.Base(tmpName), activeFile); err != nil {
		return err
	}
	syncDirBestEffort(settings)
	return nil
}

func syncDirBestEffort(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// resolveManagedStartupPath prepares the managed app-data directory and
// resolves which capture file should be opened on startup.
func resolveManagedStartupPath(dir string) (dbPath string, name string, err error) {
	settings, err := ensureManagedDirs(dir)
	if err != nil {
		return "", "", err
	}
	selected, err := readSelection(settings)
	if err != nil {
		return "", "", err
	}
	if selected == "" {
		targetName := defaultSessionName
		if canonical, found, findErr := findSessionEntry(dir, defaultSessionName); findErr != nil {
			return "", "", findErr
		} else if found {
			targetName = canonical
		}
		dbPath = filepath.Join(dir, targetName)
		if info, statErr := os.Lstat(dbPath); statErr == nil {
			if !info.Mode().IsRegular() {
				return "", "", fmt.Errorf("database is not a regular file: %s", dbPath)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", statErr
		}
		return dbPath, targetName, nil
	}
	canonical, found, findErr := findSessionEntry(dir, selected)
	if findErr != nil {
		return "", "", findErr
	}
	if !found {
		return "", "", fmt.Errorf("remembered database %s is missing from %s (restore it or remove %s to start fresh)",
			selected, dir, filepath.Join(settings, activeFile))
	}
	dbPath = filepath.Join(dir, canonical)
	info, statErr := os.Lstat(dbPath)
	if statErr != nil {
		return "", "", statErr
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("database is not a regular file: %s", dbPath)
	}
	return dbPath, canonical, nil
}

type managedSessionManager struct {
	mu    sync.Mutex
	dir   string
	store *store.Store
}

func newManagedSessionManager(dir string, s *store.Store) *managedSessionManager {
	return &managedSessionManager{dir: dir, store: s}
}

func (m *managedSessionManager) ListDatabases(_ context.Context) (server.DatabaseList, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return server.DatabaseList{}, fmt.Errorf("list managed databases: %w", err)
	}
	activeName := filepath.Base(m.store.DBPath())
	type timedEntry struct {
		entry   server.DatabaseEntry
		modTime time.Time
	}
	items := make([]timedEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !isSessionFilename(name) {
			continue
		}
		fullPath := filepath.Join(m.dir, name)
		info, err := os.Lstat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		size := info.Size()
		modTime := info.ModTime()
		if walInfo, walErr := os.Stat(fullPath + ".wal"); walErr == nil && walInfo.Mode().IsRegular() {
			size += walInfo.Size()
			if walInfo.ModTime().After(modTime) {
				modTime = walInfo.ModTime()
			}
		}
		items = append(items, timedEntry{
			entry: server.DatabaseEntry{
				Name:       name,
				SizeBytes:  size,
				ModifiedAt: modTime.UTC().Format(time.RFC3339),
				Active:     name == activeName,
			},
			modTime: modTime,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].entry.Active != items[j].entry.Active {
			return items[i].entry.Active
		}
		if !items[i].modTime.Equal(items[j].modTime) {
			return items[i].modTime.After(items[j].modTime)
		}
		return items[i].entry.Name < items[j].entry.Name
	})
	databases := make([]server.DatabaseEntry, len(items))
	for i, item := range items {
		databases[i] = item.entry
	}
	return server.DatabaseList{
		Managed:   true,
		Active:    activeName,
		Databases: databases,
	}, nil
}

func (m *managedSessionManager) OpenDatabase(ctx context.Context, name string) (server.DatabaseList, error) {
	if !isSessionFilename(name) {
		return server.DatabaseList{}, fmt.Errorf("%w: %q must be a .duckdb filename in the app data folder",
			server.ErrInvalidDatabaseName, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	canonical, found, err := findSessionEntry(m.dir, name)
	if err != nil {
		return server.DatabaseList{}, fmt.Errorf("inspect managed databases: %w", err)
	}
	if !found {
		return server.DatabaseList{}, fmt.Errorf("%w: %s", store.ErrDatabaseNotFound, name)
	}
	target := filepath.Join(m.dir, canonical)
	if err := m.store.OpenDatabase(ctx, target, func() error {
		return writeSelection(m.dir, canonical)
	}); err != nil {
		return server.DatabaseList{}, err
	}
	return m.ListDatabases(ctx)
}

func (m *managedSessionManager) NewDatabase(ctx context.Context, rawName string) (server.DatabaseList, error) {
	name, err := normalizeNewSessionName(rawName)
	if err != nil {
		return server.DatabaseList{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, found, err := findSessionEntry(m.dir, name); err != nil {
		return server.DatabaseList{}, fmt.Errorf("inspect managed databases: %w", err)
	} else if found {
		return server.DatabaseList{}, fmt.Errorf("%w: %s", store.ErrDatabaseAlreadyExists, existing)
	}
	target := filepath.Join(m.dir, name)
	if err := m.store.CreateDatabase(ctx, target, func() error {
		return writeSelection(m.dir, name)
	}); err != nil {
		return server.DatabaseList{}, err
	}
	return m.ListDatabases(ctx)
}

// staticDatabaseManager reports read-only session metadata for explicit --db
// runs, refusing managed switches.
type staticDatabaseManager struct {
	active string
}

func (m staticDatabaseManager) ListDatabases(_ context.Context) (server.DatabaseList, error) {
	return server.DatabaseList{
		Managed:   false,
		Active:    m.active,
		Databases: []server.DatabaseEntry{},
	}, nil
}

func (staticDatabaseManager) OpenDatabase(context.Context, string) (server.DatabaseList, error) {
	return server.DatabaseList{}, store.ErrDatabaseSwitchUnsupported
}

func (staticDatabaseManager) NewDatabase(context.Context, string) (server.DatabaseList, error) {
	return server.DatabaseList{}, store.ErrDatabaseSwitchUnsupported
}

// ensure fs import is used cleanly across platforms.
var _ fs.FileInfo
