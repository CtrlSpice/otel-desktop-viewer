package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExperimentCapabilitiesAndGate(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "opted in"}[enabled], func(t *testing.T) {
			file := ""
			if enabled {
				file = filepath.Join(t.TempDir(), "flags.json")
				require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0600))
			}
			flags, err := features.New(t.Context(), file)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, flags.Shutdown(context.Background())) })
			s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil, WithFeatures(flags))
			require.NoError(t, err)
			response := httptest.NewRecorder()
			s.server.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/features", nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			value := "false"
			if enabled {
				value = "true"
			}
			require.JSONEq(t, `{"evals":{"enabled":`+value+`,"label":"Experimental and unsupported"}}`, response.Body.String())
			called := false
			action := s.requireEvals(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			response = httptest.NewRecorder()
			action.ServeHTTP(response, httptest.NewRequest("POST", "/api/evals/run", nil))
			require.Equal(t, enabled, called)
			if !enabled {
				require.Equal(t, http.StatusNotFound, response.Code)
				for _, path := range []string{"/evals", "/evals/run", "/api/evals", "/api/evals/run"} {
					response = httptest.NewRecorder()
					s.server.Handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
					require.Equal(t, http.StatusNotFound, response.Code, path)
				}
			}
		})
	}
}
