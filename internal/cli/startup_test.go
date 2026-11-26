package cli

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOccupiedOTLPPortPreservesDatabaseSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	dir := filepath.Join(home, "otel-desktop-viewer")
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "com.ctrlspice.otel-desktop-viewer")
	case "windows":
		dir = filepath.Join(home, "OTel Desktop Viewer")
	}
	settings := filepath.Join(dir, "settings")
	require.NoError(t, os.MkdirAll(settings, 0700))
	marker := filepath.Join(settings, "active-database")
	require.NoError(t, os.WriteFile(marker, []byte("previous.duckdb"), 0600))

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer busy.Close()
	target := filepath.Join(dir, "next.duckdb")
	cmd := newCommand(collector.Settings("test"))
	cmd.SetArgs([]string{"--db", target, "--host", "127.0.0.1", "--open-browser=false",
		"--browser-port", "0", "--grpc", "0", "--http", strconv.Itoa(busy.Addr().(*net.TCPAddr).Port)})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	err = cmd.ExecuteContext(ctx)
	require.ErrorContains(t, err, busy.Addr().String())
	require.FileExists(t, target) // The database opened before the receiver failed.
	selected, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "previous.duckdb", string(selected))
}

// TestStartupFailureIsNotAnsweredWithUsage distinguishes command errors from
// collector startup failures. Usage remains enabled until flag parsing succeeds.
func TestStartupFailureIsNotAnsweredWithUsage(t *testing.T) {
	set := collector.Settings("test")

	t.Run("a bad flag still prints usage", func(t *testing.T) {
		cmd := newCommand(set)
		cmd.SetArgs([]string{"--nonsense-flag"})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, out.String(), "USAGE",
			"a mistyped flag is exactly what usage exists to explain")
	})

	// RunE must enable usage silencing; its initial value alone proves nothing.
	t.Run("reaching RunE arms the silencing", func(t *testing.T) {
		cmd := newCommand(set)
		require.False(t, cmd.SilenceUsage,
			"not on the command, or a mistyped flag would be silenced too")
		require.True(t, cmd.SilenceErrors,
			"the process boundary prints returned errors once")

		// Enter RunE with a config that cannot start.
		cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		_ = cmd.Execute()

		assert.True(t, cmd.SilenceUsage,
			"RunE must silence usage: past flag parsing, a failure is not a usage mistake")
		assert.True(t, cmd.SilenceErrors)
		assert.NotContains(t, out.String(), "USAGE",
			"a startup failure must not be answered with the flag listing")
	})
}
