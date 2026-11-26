package store

import "golang.org/x/sys/unix"

func publishNewDatabase(staged, path string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, staged, unix.AT_FDCWD, path, unix.RENAME_EXCL)
}
