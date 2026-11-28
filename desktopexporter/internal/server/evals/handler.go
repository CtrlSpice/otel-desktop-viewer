//go:build evals

package evals

import (
	"database/sql"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

// Handler serves the Evals HTTP API and manages pre-redaction receiver forwarding.
type Handler struct {
	store          *store.Store
	imports        sync.Map
	evidenceLock   chan struct{}
	receiverURL    string
	receiverClient *http.Client
}

func New(st *store.Store, host string, otlpHTTPPort int) *Handler {
	return &Handler{
		store:          st,
		receiverClient: newReceiverClient(),
		evidenceLock:   make(chan struct{}, 1),
		receiverURL:    "http://" + net.JoinHostPort(host, strconv.Itoa(otlpHTTPPort)) + "/v1/logs",
	}
}

func (h *Handler) Close() {
	if h == nil || h.receiverClient == nil {
		return
	}
	h.receiverClient.CloseIdleConnections()
}

func (h *Handler) Register(mux *http.ServeMux, spa http.Handler) {
	mux.Handle("/evals", spa)
	mux.Handle("/evals/", spa)
	mux.HandleFunc("/api/evals", http.NotFound)
	mux.HandleFunc("/api/evals/", http.NotFound)
	mux.HandleFunc("POST /api/evals/ingest", h.ingestEvals)
	mux.HandleFunc("GET /api/evals/experiments", h.searchEvalExperiments)
	mux.HandleFunc("GET /api/evals/experiments/{id}/rows", h.getEvalExperimentPage)
}

func storeRead[T any](st *store.Store, read func(*sql.DB) (T, error)) (T, error) {
	var result T
	err := st.WithDBRead(func(db *sql.DB) error {
		var err error
		result, err = read(db)
		return err
	})
	return result, err
}
