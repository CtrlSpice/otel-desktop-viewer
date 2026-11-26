package store

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishNewDatabase(staged, path string) error {
	err := unix.Renameat2(unix.AT_FDCWD, staged, unix.AT_FDCWD, path, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return os.Link(staged, path)
	}
	return err
}
