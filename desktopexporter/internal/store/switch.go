package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/commitbridge"
	"go.uber.org/zap"
)

// OpenDatabase switches the store to an existing database file by checkpointing
// and closing the active DuckDB instance, then opening targetPath cleanly.
// onActivated runs after the target database opens; if it fails, the previous
// database is reopened. onActivated must not call back into Store.
func (s *Store) OpenDatabase(ctx context.Context, targetPath string, onActivated func() error) error {
	return s.switchDatabase(ctx, targetPath, false, onActivated)
}

// CreateDatabase creates a new database file without overwriting an existing
// one, switches the store to it, and runs onActivated before exposing it.
// onActivated must not call back into Store.
func (s *Store) CreateDatabase(ctx context.Context, targetPath string, onActivated func() error) error {
	return s.switchDatabase(ctx, targetPath, true, onActivated)
}

func (s *Store) switchDatabase(ctx context.Context, targetPath string, create bool, onActivated func() error) error {
	if s.inMemory {
		return ErrDatabaseSwitchUnsupported
	}
	targetPath = filepath.Clean(targetPath)
	if !filepath.IsAbs(targetPath) || strings.Contains(targetPath, "?") {
		return fmt.Errorf("invalid database path: %q", targetPath)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.db == nil {
		return ErrStoreConnectionClosed
	}

	if create {
		if err := checkCreateTargetAbsent(targetPath); err != nil {
			return err
		}
	} else {
		info, err := os.Lstat(targetPath)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrDatabaseNotFound, filepath.Base(targetPath))
		}
		if err != nil {
			return fmt.Errorf("inspect database: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("database is not a regular file: %s", filepath.Base(targetPath))
		}
		if sameFilePath(targetPath, s.dbPath) {
			if onActivated != nil {
				return onActivated()
			}
			return nil
		}
	}

	oldPath := s.dbPath
	oldCommits := s.commits
	var oldRev uint64
	if oldCommits != nil {
		oldRev, _, _ = oldCommits.Snapshot()
	}

	// Flush WAL into the main database file so switching away leaves a
	// clean, self-contained .duckdb file on disk.
	if _, err := s.db.ExecContext(context.Background(), "checkpoint"); err != nil {
		s.logger.Warn("checkpoint before database switch failed",
			zap.String("database", oldPath),
			zap.Error(err))
	}
	if err := s.databaseHandles.closeDB(); err != nil {
		s.logger.Warn("closing active database before switch returned error",
			zap.String("database", oldPath),
			zap.Error(err))
	}

	if create {
		if beforeCreateOpenHook != nil {
			beforeCreateOpenHook()
		}
		// Re-check immediately before opening so a file created by another
		// process during checkpoint/close is rejected without unlinking it.
		if err := checkCreateTargetAbsent(targetPath); err != nil {
			if restoreErr := s.restorePrevious(oldPath, oldCommits, oldRev); restoreErr != nil {
				return errors.Join(err, restoreErr)
			}
			return err
		}
		if beforeCreateConnectorHook != nil {
			beforeCreateConnectorHook()
		}
	}

	next, opened, err := openDatabaseHandles(ctx, targetPath, s.logger)
	if err != nil {
		// Only remove artifacts when we verified the destination was absent
		// right before open AND DuckDB actually acquired/created it. If
		// NewConnector failed (e.g. another process raced and locked the file),
		// never unlink the destination.
		if create && opened {
			removeDatabaseArtifacts(targetPath)
		}
		if restoreErr := s.restorePrevious(oldPath, oldCommits, oldRev); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		return err
	}

	if onActivated != nil {
		if err := onActivated(); err != nil {
			_ = next.closeAll()
			if create && opened {
				removeDatabaseArtifacts(targetPath)
			}
			if restoreErr := s.restorePrevious(oldPath, oldCommits, oldRev); restoreErr != nil {
				return errors.Join(err, restoreErr)
			}
			return err
		}
	}

	s.databaseHandles = next
	s.revisionBase += oldRev + 1
	if oldCommits != nil {
		oldCommits.Close()
	}
	return nil
}

// Test hooks for deterministically exercising TOCTOU races during CreateDatabase.
var (
	beforeCreateOpenHook      func()
	beforeCreateConnectorHook func()
)

func checkCreateTargetAbsent(targetPath string) error {
	for _, candidate := range []string{targetPath, targetPath + ".wal", targetPath + ".tmp"} {
		_, err := os.Lstat(candidate)
		if err == nil {
			return fmt.Errorf("%w: %s", ErrDatabaseAlreadyExists, filepath.Base(candidate))
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect destination database: %w", err)
		}
	}
	return nil
}

func (s *Store) restorePrevious(oldPath string, oldCommits *commitbridge.Observer, oldRev uint64) error {
	restored, _, err := openDatabaseHandles(context.Background(), oldPath, s.logger)
	if err != nil {
		s.closed = true
		if oldCommits != nil {
			oldCommits.Close()
		}
		s.commits = nil
		return fmt.Errorf("restore previous database %s: %w", oldPath, err)
	}
	s.databaseHandles = restored
	s.revisionBase += oldRev + 1
	if oldCommits != nil {
		oldCommits.Close()
	}
	return nil
}

func sameFilePath(a, b string) bool {
	if a == b {
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

func removeDatabaseArtifacts(dbPath string) {
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + ".wal")
	_ = os.RemoveAll(dbPath + ".tmp")
}
