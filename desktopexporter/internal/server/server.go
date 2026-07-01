package server

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/rs/cors"
	"golang.org/x/exp/jsonrpc2"
)

//go:embed static
var assets embed.FS

type Server struct {
	server         http.Server
	jsonrpcHandler *JSONRPCHandler
	staticDir      string
}

func NewServer(endpoint string, store *store.Store) (*Server, error) {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		jsonrpcHandler: NewJSONRPCHandler(store),
		staticDir:      getStaticDir(),
	}

	if err := s.initHandler(); err != nil {
		return nil, fmt.Errorf("could not initialize desktop exporter server: %w", err)
	}

	return &s, nil
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) initHandler() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rpc", s.rpcHandler)

	// Single-page app: serve a static asset when one exists at the request path,
	// otherwise fall back to index.html so client-side routes (/traces,
	// /traces/{id}, /metrics, /logs) resolve on hard load, refresh, and shared links.
	fsys, err := s.staticFS()
	if err != nil {
		return err
	}
	mux.Handle("/", spaHandler(fsys))

	// CORS for the Vite frontend
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://*", "https://*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})
	s.server.Handler = c.Handler(mux)
	return nil
}

// staticFS returns the filesystem holding the built frontend: the on-disk
// STATIC_ASSETS_DIR in development, or the embedded assets in production.
func (s *Server) staticFS() (fs.FS, error) {
	if s.staticDir != "" {
		return os.DirFS(s.staticDir), nil
	}
	return fs.Sub(assets, "static")
}

// spaHandler serves the single-page app. A GET/HEAD for an existing file is served
// as-is; any other GET/HEAD path falls back to index.html so the client router owns
// the route (e.g. a deep-linked /traces/{id} or a refreshed /metrics). Non-GET
// requests fall through to the file server.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	serveIndex := func(writer http.ResponseWriter) {
		bytes, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			log.Printf("Error reading index.html: %v", err)
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
				serveIndex(writer)
				return
			}
		}
		fileServer.ServeHTTP(writer, request)
	})
}

func (s *Server) rpcHandler(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrInternal)
		return
	}

	message, err := jsonrpc2.DecodeMessage(body)
	if err != nil {
		sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrParse)
		return
	}

	rpcRequest, ok := message.(*jsonrpc2.Request)
	if !ok {
		sendJSONRPCResponse(writer, jsonrpc2.ID{}, nil, jsonrpc2.ErrInvalidRequest)
		return
	}

	result, err := s.jsonrpcHandler.Handle(request.Context(), rpcRequest)
	if err != nil {
		sendJSONRPCResponse(writer, rpcRequest.ID, nil, err)
		return
	}

	sendJSONRPCResponse(writer, rpcRequest.ID, result, nil)
}

// sendJSONRPCResponse encodes and writes a spec-compliant JSON-RPC response body
// It handles both success and error cases, with fallback http response
// if our error handling creates new and exciting errors
func sendJSONRPCResponse(writer http.ResponseWriter, id jsonrpc2.ID, result any, rpcError error) {
	response, err := jsonrpc2.NewResponse(id, result, rpcError)
	if err != nil {
		log.Printf("Error creating JSON-RPC response: %v", err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}

	bytes, err := jsonrpc2.EncodeMessage(response)
	if err != nil {
		log.Printf("Error encoding JSON-RPC response: %v", err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(bytes)
}

// getStaticDir returns the path to the static assets directory for development.
// Set STATIC_ASSETS_DIR to point at the frontend build output (e.g. frontend/dist).
// Returns empty string if not set (uses embedded assets).
func getStaticDir() string {
	if dir, ok := os.LookupEnv("STATIC_ASSETS_DIR"); ok {
		return filepath.Clean(dir)
	}
	return ""
}

func (s *Server) Close() error {
	return s.server.Close()
}
