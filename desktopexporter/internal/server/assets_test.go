package server

import (
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedFrontendMatchesBuild(t *testing.T) {
	assets, err := staticFS()
	require.NoError(t, err)
	data, err := fs.ReadFile(assets, "build-features.json")
	require.NoError(t, err, "rebuild the embedded frontend for this build variant")
	var manifest struct {
		Evals bool `json:"evals"`
	}
	require.NoError(t, json.Unmarshal(data, &manifest))
	require.Equal(t, features.EvalsBuilt, manifest.Evals)
}
