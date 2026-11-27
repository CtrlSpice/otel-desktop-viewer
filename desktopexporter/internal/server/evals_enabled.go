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
	mux.Handle("/evals", spa)
	mux.Handle("/evals/", spa)
	mux.HandleFunc("/api/evals", http.NotFound)
	mux.HandleFunc("/api/evals/", http.NotFound)
	mux.HandleFunc("POST /api/evals/ingest", s.ingestEvals)
	mux.HandleFunc("PUT /api/evals/runs/{runID}/results/{resultID}", s.putEvalResult)
	mux.HandleFunc("GET /api/evals/experiments", s.searchEvalExperiments)
	mux.HandleFunc("GET /api/evals/experiments/{id}/rows", s.getEvalExperimentPage)
}
