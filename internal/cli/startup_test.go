package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
