package server

import (
	"encoding/json"
	"net/http"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
)

type featureState struct {
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
}

// featureConfig reports the capabilities compiled into this viewer.
func (s *Server) featureConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Evals featureState `json:"evals"`
	}{Evals: featureState{
		Enabled: features.EvalsBuilt,
		Label:   features.ExperimentalLabel,
	}})
}
