//go:build !evals

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type unreadEvalBody struct{ t *testing.T }

func (b unreadEvalBody) Read([]byte) (int, error) {
	b.t.Error("public build read an Evals request body")
	return 0, io.EOF
}

func TestPublicBuildHasNoEvalsRoutes(t *testing.T) {
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
		for _, path := range []string{"/evals", "/evals/example", "/api/evals", "/api/evals/ingest", "/api/evals/experiments", "/api/evals/experiments/example/rows"} {
			response := httptest.NewRecorder()
			s.server.Handler.ServeHTTP(response, httptest.NewRequest(method, path, unreadEvalBody{t}))
			require.Equal(t, http.StatusNotFound, response.Code, "%s %s", method, path)
		}
	}
	response := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/features", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"evals":{"enabled":false,"label":"Experimental and unsupported"}}`, response.Body.String())
}
