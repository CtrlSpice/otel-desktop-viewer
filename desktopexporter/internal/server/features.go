package server

import (
	"encoding/json"
	"net/http"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
)

type Option func(*Server)

type featureState struct {
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
}

func WithFeatures(flags *features.Flags) Option {
	return func(s *Server) { s.features = flags }
}

// featureConfig is the browser's capability source. Experimental status is
// product policy, not user-controlled flag metadata.
func (s *Server) featureConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Evals featureState `json:"evals"`
	}{Evals: featureState{
		Enabled: s.features.EvalsEnabled(r.Context()),
		Label:   features.ExperimentalLabel,
	}})
}

// requireEvals gates both page requests and HTTP actions. Future eval RPC
// methods must also check EvalsEnabled before starting work.
func (s *Server) requireEvals(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.features.EvalsEnabled(r.Context()) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
