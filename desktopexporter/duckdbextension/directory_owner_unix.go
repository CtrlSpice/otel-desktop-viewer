//go:build !windows

package duckdbextension

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func checkDirectoryOwner(dir string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify owner of %s", dir)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("expected app data directory %s to be owned by current user", dir)
	}
	return nil
}
