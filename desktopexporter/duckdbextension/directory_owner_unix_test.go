//go:build unix

package duckdbextension

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedDirectoryOwner(t *testing.T) {
	dir := t.TempDir()
	info, err := os.Lstat(dir)
	require.NoError(t, err)
	require.NoError(t, checkDirectoryOwner(dir, info))

	// Change only the in-memory stat snapshot. No privileges or chown needed.
	info.Sys().(*syscall.Stat_t).Uid ^= 1
	require.ErrorContains(t, checkDirectoryOwner(dir, info), "expected collector UID")
}
