//go:build evals

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEvalsBuildExposesWorkspace(t *testing.T) {
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/features", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.JSONEq(t, `{"evals":{"enabled":true,"label":"Experimental and unsupported"}}`, response.Body.String())

	response = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/evals", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
}
