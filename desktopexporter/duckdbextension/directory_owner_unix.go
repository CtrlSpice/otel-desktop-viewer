//go:build unix

package duckdbextension

import (
	"fmt"
	"os"
	"syscall"
)

func checkDirectoryOwner(dir string, info os.FileInfo) error {
	owner := info.Sys().(*syscall.Stat_t).Uid
	if int64(owner) != int64(os.Geteuid()) {
		return fmt.Errorf("app data directory %s belongs to UID %d; expected collector UID %d", dir, owner, os.Geteuid())
	}
	return nil
}
