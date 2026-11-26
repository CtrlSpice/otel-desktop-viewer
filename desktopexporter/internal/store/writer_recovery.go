package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DuckDB identifies the main file by handle but locates its WAL by pathname.
// Hold the same per-user, per-file lock across replay, handoff and normal use.
// The lock file stays in place so concurrent openers always lock the same inode.
type writerRecord struct {
	lock      *os.File
	path      string
	replaying string
	current   string
	requested string // canonical name returned by the requested attachment's guard
}

func (f *inspectedFile) recoverWriter(ctx context.Context) error {
	if f.engine.files.RecoveryDir == "" {
		return f.guardRequested(ctx)
	}
	previous, err := f.beginWriterRecovery(ctx)
	if err != nil {
		return err
	}
	if f.writer.replaying != "" {
		if _, err := f.engine.control.ExecContext(ctx, "checkpoint "+f.catalog); err != nil {
			return err
		}
		if err := f.detach(); err != nil {
			return err
		}
		if err := f.unguard(); err != nil {
			return err
		}
		f.writer.replaying = ""
		if pending, err := hasRecoveryFiles(previous); err != nil || pending {
			return errors.Join(fmt.Errorf("journal recovery did not finish at %s", previous), err)
		}
	}
	if !f.guarded {
		if err := f.guardRequested(ctx); err != nil {
			return err
		}
	}
	id, err := identifyFile(f.writer.requested)
	if err != nil || id != f.fileIdentity {
		return errors.Join(fmt.Errorf("database file changed before recording journal path: %s", f.writer.requested), err)
	}
	return f.writer.remember(previous, f.writer.requested)
}

// A process that exits in this step leaves the previous writer recorded.
func (f *inspectedFile) beginWriterRecovery(ctx context.Context) (string, error) {
	dir := f.engine.files.RecoveryDir
	record := filepath.Join(dir, fmt.Sprintf("%x-%x", f.device, f.id))
	lock, err := os.OpenFile(record+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	if err := lockWriterRecord(lock); err != nil {
		lock.Close()
		return "", fmt.Errorf("lock recovery record for %s: %w", f.Name(), err)
	}
	w := &writerRecord{lock: lock, path: record}
	f.writer = w
	data, err := os.ReadFile(record)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	previous := string(data)
	if err == nil && !filepath.IsAbs(previous) {
		return "", fmt.Errorf("invalid writer path in %s", record)
	}
	if err := f.guardRequested(ctx); err != nil {
		return "", err
	}
	if previous != "" && previous != w.requested {
		pending, err := hasRecoveryFiles(previous)
		if err != nil {
			return "", err
		}
		if pending {
			id, err := identifyFile(previous)
			if err != nil || id != f.fileIdentity {
				return "", errors.Join(fmt.Errorf("journal at %s does not have its original database", previous), err)
			}
			if err := f.unguard(); err != nil {
				return "", err
			}
			w.replaying = previous
			if err := f.guardName(ctx, f.engine.control, previous); err != nil {
				return "", err
			}
			f.guarded = true
			if err := f.attach(ctx, previous, ""); err != nil {
				return "", err
			}
		}
	}
	return previous, nil
}

func (w *writerRecord) remember(previous, name string) error {
	if previous != name {
		dir := filepath.Dir(w.path)
		tmp, err := os.CreateTemp(dir, ".writer-")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		_, writeErr := tmp.WriteString(name)
		if err := errors.Join(writeErr, tmp.Sync(), tmp.Close()); err != nil {
			return err
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := root.Rename(filepath.Base(tmp.Name()), filepath.Base(w.path)); err != nil {
			return err
		}
	}
	w.current = name
	return nil
}

func (w *writerRecord) release(clean bool) error {
	var err error
	if clean && w.current != "" {
		var pending bool
		pending, err = hasRecoveryFiles(w.current)
		if err == nil && !pending {
			err = os.Remove(w.path)
		}
	}
	return errors.Join(err, w.lock.Close())
}

func hasRecoveryFiles(path string) (bool, error) {
	for _, suffix := range recoverySuffixes {
		if _, err := os.Lstat(path + suffix); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
