package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestLongWindowsDatabasePath(t *testing.T) {
	dir := t.TempDir()
	for len(dir) <= 260 {
		dir = filepath.Join(dir, strings.Repeat("long", 12))
	}
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "capture.duckdb")
	for _, create := range []bool{true, false} {
		s, err := OpenViewerDatabase(t.Context(), path, create, nil)
		require.NoError(t, err)
		require.NoError(t, s.Close())
	}
	unc := `\\server\share\` + strings.Repeat(`directory\`, 35) + "capture.duckdb"
	name, err := windowsFilename(unc)
	require.NoError(t, err)
	require.Equal(t, `\\?\UNC\`+unc[2:], windows.UTF16PtrToString(name))
}
