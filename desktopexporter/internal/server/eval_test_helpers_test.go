package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/stretchr/testify/require"
)

func enabledEvalFlags(t *testing.T) *features.Flags {
	t.Helper()
	file := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0o600))
	flags, err := features.New(t.Context(), file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, flags.Shutdown(context.Background())) })
	return flags
}
