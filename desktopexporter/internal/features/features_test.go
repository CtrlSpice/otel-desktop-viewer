//go:build evals

package features

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDisabledWithoutConfiguration(t *testing.T) {
	// Ambient flagd configuration cannot opt the viewer into a remote provider.
	t.Setenv("FLAGD_RESOLVER", "rpc")
	t.Setenv("FLAGD_OFFLINE_FLAG_SOURCE_PATH", "/does/not/exist")
	f, err := New(t.Context(), "")
	require.NoError(t, err)
	require.Nil(t, f.api)
	require.False(t, f.EvalsEnabled(t.Context()))
	require.NoError(t, f.Shutdown(t.Context()))
}

func TestLocalFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   string
		enabled bool
	}{
		{"enabled", `{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}`, true},
		{"disabled", `{"evals.enabled":{"state":"ENABLED","variants":{"off":false},"defaultVariant":"off"}}`, false},
		{"missing", `{}`, false},
		{"wrong type", `{"evals.enabled":{"state":"ENABLED","variants":{"on":"true"},"defaultVariant":"on"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "flags.json")
			require.NoError(t, os.WriteFile(file, []byte(`{"flags":`+tc.flags+`}`), 0600))
			f, err := New(t.Context(), file)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, f.Shutdown(context.Background())) })
			require.Equal(t, tc.enabled, f.EvalsEnabled(t.Context()))
		})
	}
}

func TestLocalFlagReloadAndIsolation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "flags.json")
	write := func(variant string) {
		t.Helper()
		require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true,"off":false},"defaultVariant":"`+variant+`"}}}`), 0600))
	}
	write("on")
	f, err := New(t.Context(), file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Shutdown(context.Background())) })
	require.True(t, f.EvalsEnabled(t.Context()))
	other, err := New(t.Context(), "")
	require.NoError(t, err)
	require.False(t, other.EvalsEnabled(t.Context()))
	write("off")
	require.Eventually(t, func() bool { return !f.EvalsEnabled(t.Context()) }, 12*time.Second, 50*time.Millisecond)
}

func TestBadFileRejectsStartup(t *testing.T) {
	for _, data := range []string{"", `{"flags":`} {
		file := filepath.Join(t.TempDir(), "flags.json")
		if data != "" {
			require.NoError(t, os.WriteFile(file, []byte(data), 0600))
		}
		f, err := New(t.Context(), file)
		require.Error(t, err)
		require.Nil(t, f)
	}
}
