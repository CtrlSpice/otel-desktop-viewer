//go:build !evals

package features

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicBuildCannotEnableEvals(t *testing.T) {
	flags, err := New(t.Context(), "")
	require.NoError(t, err)
	require.False(t, flags.EvalsEnabled(t.Context()))
	require.NoError(t, flags.Shutdown(t.Context()))

	file := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0600))
	_, err = New(t.Context(), file)
	require.ErrorContains(t, err, "not included in this build")
}
