//go:build evals

package server

import (
	"net"
	"net/http"
	"strconv"
	"sync"
)

type evalServer struct {
	evalImports        sync.Map
	evalEvidenceLock   chan struct{}
	evalReceiverURL    string
	evalReceiverClient *http.Client
}

func newEvalServer(host string, port int) evalServer {
	return evalServer{
		evalReceiverClient: newEvalReceiverClient(),
		evalEvidenceLock:   make(chan struct{}, 1),
		evalReceiverURL:    "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/v1/logs",
	}
}

func (s *Server) closeEvals() { s.evalReceiverClient.CloseIdleConnections() }

func (s *Server) registerEvals(mux *http.ServeMux, spa http.Handler) {
	mux.Handle("/evals", s.requireEvals(spa))
	mux.Handle("/evals/", s.requireEvals(spa))
	mux.Handle("/api/evals", s.requireEvals(http.NotFoundHandler()))
	mux.Handle("/api/evals/", s.requireEvals(http.NotFoundHandler()))
	mux.HandleFunc("POST /api/evals/ingest", s.ingestEvals)
	mux.Handle("GET /api/evals/experiments", s.requireEvals(http.HandlerFunc(s.searchEvalExperiments)))
	mux.Handle("GET /api/evals/experiments/{id}/rows", s.requireEvals(http.HandlerFunc(s.getEvalExperimentPage)))
}
