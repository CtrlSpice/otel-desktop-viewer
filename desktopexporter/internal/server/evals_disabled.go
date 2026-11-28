//go:build !evals

package server

import (
	"net/http"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

type evalServer struct{}

func newEvalServer(*store.Store, string, int) evalServer { return evalServer{} }
func (*Server) closeEvals()                              {}

func (*Server) registerEvals(mux *http.ServeMux, _ http.Handler) {
	// Keep extensionless Evals URLs out of the shared SPA fallback.
	for _, path := range []string{"/evals", "/evals/", "/api/evals", "/api/evals/"} {
		mux.Handle(path, http.NotFoundHandler())
	}
}
