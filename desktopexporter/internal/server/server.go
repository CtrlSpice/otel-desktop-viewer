package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

//go:embed static
var assets embed.FS

// maxRPCRequestBodyBytes caps how much of an /rpc request body we will read.
// JSON-RPC requests in this app are small (a method name and a few params), so
// 1 MB is generous headroom while preventing a buggy or malicious client from
// exhausting memory with an unbounded body.
const maxRPCRequestBodyBytes = 1 << 20 // 1 MB

type Server struct {
	server         http.Server
	jsonrpcHandler *JSONRPCHandler
	logger         *zap.Logger

	serveDone chan struct{}
	startMu   sync.Mutex
}

func NewServer(endpoint string, store *store.Store, logger *zap.Logger) (*Server, error) {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		jsonrpcHandler: NewJSONRPCHandler(store, logger),
		logger:         logger,
	}

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

	// Single-page app: serve a static asset when one exists at the request path,
	// otherwise fall back to index.html so client-side routes (/traces,
	// /traces/{id}, /metrics, /logs) resolve on hard load, refresh, and shared links.
	fsys, err := staticFS()
	if err != nil {
		return err
	}
	mux.Handle("/", spaHandler(fsys, s.logger))

	// CORS for the Vite frontend
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://*", "https://*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})
	s.server.Handler = c.Handler(mux)
	return nil
}

func staticFS() (fs.FS, error) {
	return fs.Sub(assets, "static")
}

// spaHandler serves the single-page app. A GET/HEAD for an existing file is served
// as-is. Extension-less paths with no matching file fall back to index.html so the
// client router owns the route (e.g. a deep-linked /traces/{id} or a refreshed
// /metrics). Paths with a file extension that do not exist 404 normally. Non-GET
// requests fall through to the file server.
func spaHandler(fsys fs.FS, logger *zap.Logger) http.Handler {
	fileServer := http.FileServerFS(fsys)
	serveIndex := func(writer http.ResponseWriter) {
		bytes, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			logger.Error("reading index.html", zap.Error(err))
			http.Error(writer, "Failed to load page", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write(bytes)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet || request.Method == http.MethodHead {
			name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
			if info, err := fs.Stat(fsys, name); name == "" || err != nil || info.IsDir() {
				if path.Ext(name) == "" {
					serveIndex(writer)
					return
				}
			}
		}
		fileServer.ServeHTTP(writer, request)
	})
}

func (s *Server) rpcHandler(writer http.ResponseWriter, request *http.Request) {
	limited := http.MaxBytesReader(writer, request.Body, maxRPCRequestBodyBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			s.sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, fmt.Errorf("%w: request body too large", jsonrpc2.ErrInvalidRequest))
			return
		}
		s.sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrInternal)
		return
	}

	message, err := jsonrpc2.DecodeMessage(body)
	if err != nil {
		s.sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrParse)
		return
	}

	rpcRequest, ok := message.(*jsonrpc2.Request)
	if !ok {
		s.sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrInvalidRequest)
		return
	}

	result, err := s.jsonrpcHandler.Handle(request.Context(), rpcRequest)
	if err != nil {
		s.sendJSONRPCResponse(writer, rpcRequest.ID, nil, err)
		return
	}

	s.sendJSONRPCResponse(writer, rpcRequest.ID, result, nil)
}

func (s *Server) sendJSONRPCResponse(writer http.ResponseWriter, id jsonrpc2.ID, result any, rpcError error) {
	response, err := jsonrpc2.NewResponse(id, result, rpcError)
	if err != nil {
		s.logger.Error("creating JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}

	bytes, err := jsonrpc2.EncodeMessage(response)
	if err != nil {
		s.logger.Error("encoding JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(bytes)
}
