package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

type Server struct {
	evalServer
	server         http.Server
	store          *store.Store
	jsonrpcHandler *JSONRPCHandler
	logger         *zap.Logger
	tel            *telemetry.Telemetry

	serveDone   chan struct{}
	startMu     sync.Mutex
	streamDone  chan struct{}
	stopStreams sync.Once
}

type Option func(*Server)

// WithOTLPGRPCPort supplies the receiver port advertised by the setup guide.
func WithOTLPGRPCPort(port int) Option {
	return func(s *Server) { s.jsonrpcHandler.otlpGRPCPort = port }
}

func NewServer(endpoint string, otlpHTTPPort int, store *store.Store, logger *zap.Logger, tel *telemetry.Telemetry, options ...Option) (*Server, error) {
	if tel == nil {
		tel = telemetry.Disabled()
	}
	s := Server{
		server: http.Server{
			Addr: endpoint,

			// ReadHeaderTimeout and IdleTimeout are pure hygiene: they bound a
			// client that opens a connection and dribbles (or never sends)
			// headers, and reap idle keep-alives. Neither can affect a
			// legitimate request.
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,

			// Deliberately no WriteTimeout. Go measures it from the end of
			// header read through the end of the response write, so it caps
			// *handler execution* too -- and a getTraceView over a large trace
			// is unbounded by design. A WriteTimeout here would abort exactly
			// the slow queries a trace viewer exists to serve, and would do it
			// by killing the connection with a partial body. Query cost is
			// bounded by cancellation instead: the client aborts, and the
			// context tears the DuckDB query down.
		},
		jsonrpcHandler: NewJSONRPCHandler(store, logger),
		store:          store,
		logger:         logger,
		tel:            tel,
		streamDone:     make(chan struct{}),
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid viewer endpoint: %w", err)
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	s.evalServer = newEvalServer(host, otlpHTTPPort)

	for _, option := range options {
		option(&s)
	}
	s.jsonrpcHandler.otlpHTTPPort = otlpHTTPPort
	if err := s.initHandler(); err != nil {
		return nil, fmt.Errorf("could not initialize desktop exporter server: %w", err)
	}

	return &s, nil
}

// Start binds the listen address synchronously, then serves HTTP on a
// background goroutine. Bind failures (e.g. port in use) return immediately.
func (s *Server) Start() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.serveDone != nil {
		return errors.New("server already started")
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	s.serveDone = done
	go func() {
		defer close(done)
		err := s.server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()
	return nil
}

// Shutdown drains in-flight requests and waits for the serve goroutine to exit.
func (s *Server) Shutdown(ctx context.Context) error {
	defer s.closeEvals()
	s.stopStreams.Do(func() { close(s.streamDone) })
	s.startMu.Lock()
	done := s.serveDone
	s.startMu.Unlock()

	if done == nil {
		return nil
	}

	err := s.server.Shutdown(ctx)
	<-done
	return err
}

func (s *Server) initHandler() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rpc", s.rpcHandler)
	mux.HandleFunc("POST /api/data/{method}", s.arrowHandler)
	mux.HandleFunc("GET /api/features", s.featureConfig)
	mux.HandleFunc("GET /api/changes", s.changesHandler)
	mux.HandleFunc("POST /api/import-target/{signal}", s.importTargetHandler)
	discovery, err := openRPCHandler()
	if err != nil {
		return err
	}
	mux.Handle("GET /openrpc.json", discovery)
	httpDiscovery, err := openAPIHandler()
	if err != nil {
		return err
	}
	mux.Handle("GET /openapi.json", httpDiscovery)
	mux.Handle("GET /llms.txt", llmsTextHandler())
	mux.HandleFunc("GET /export/{signal}/{id}", s.exportHandler)

	// Single-page app: serve a static asset when one exists at the request path,
	// otherwise fall back to index.html so client-side routes (/traces,
	// /traces/{id}, /metrics, /logs) resolve on hard load, refresh, and shared links.
	fsys, err := staticFS()
	if err != nil {
		return err
	}
	spa := spaHandler(fsys, s.logger)
	mux.Handle("/", spa)
	s.registerEvals(mux, spa)

	// CORS for the Vite frontend
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://*", "https://*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{"X-Arrow-Rows"},
	})
	s.server.Handler = c.Handler(mux)
	return nil
}
