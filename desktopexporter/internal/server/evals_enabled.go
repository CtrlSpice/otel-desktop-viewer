//go:build evals

package server

import (
	"net/http"

	serverevals "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

type evalServer struct {
	evals *serverevals.Handler
}

func newEvalServer(st *store.Store, host string, port int) evalServer {
	return evalServer{evals: serverevals.New(st, host, port)}
}

func (s *Server) closeEvals() { s.evals.Close() }

func (s *Server) registerEvals(mux *http.ServeMux, spa http.Handler) {
	s.evals.Register(mux, spa)
}
