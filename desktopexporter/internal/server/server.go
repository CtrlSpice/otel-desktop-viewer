package server

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/rs/cors"
	"golang.org/x/exp/jsonrpc2"
)

//go:embed static/*
var assets embed.FS

//go:embed static-v2/*
var assetsV2 embed.FS

type Server struct {
	server         http.Server
	jsonrpcHandler *JSONRPCHandler
	staticDir      string
	useV2Frontend  bool // Feature flag for v2 frontend
}

func NewServer(endpoint string, store *store.Store) *Server {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		jsonrpcHandler: NewJSONRPCHandler(store),
		staticDir:      getStaticDir(),
		useV2Frontend:  getFeatureFlag(),
	}

	if err := s.initHandler(); err != nil {
		log.Fatalf("Could not initialize desktop exporter server: %v", err)
	}

	return &s
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) initHandler() error {
	mux := http.NewServeMux()

	// Handle specific routes first (checked before catch-all)
	mux.HandleFunc("GET /traces/{id}", s.indexHandler)
	mux.HandleFunc("POST /rpc", s.rpcHandler)

	// Then handle static files (catches everything else)
	if s.staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.staticDir)))
	} else {
		// Serve v1 static assets by default
		staticContent, err := fs.Sub(assets, "static")
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle("/", http.FileServerFS(staticContent))
	}

	// CORS for the Vite frontend
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://*", "https://*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})
	s.server.Handler = c.Handler(mux)
	return nil
}

// indexHandler serves the frontend application
// It handles both development (staticDir) and production (embedded assets) scenarios
// Uses environment variable USE_V2_FRONTEND to switch between v1 and v2 frontends
func (s *Server) indexHandler(writer http.ResponseWriter, request *http.Request) {
	// Use the configured feature flag from environment variable
	if s.useV2Frontend {
		s.serveV2Frontend(writer, request)
	} else {
		s.serveV1Frontend(writer, request)
	}
}

// serveV1Frontend serves the current (v1) frontend
func (s *Server) serveV1Frontend(writer http.ResponseWriter, request *http.Request) {
	if s.staticDir != "" {
		http.ServeFile(writer, request, s.staticDir+"/index.html")
	} else {
		bytes, err := assets.ReadFile("static/index.html")
		if err != nil {
			log.Printf("Error reading static assets: %v", err)
			http.Error(writer, "Failed to load page", http.StatusInternalServerError)
			return
		}
		writer.Write(bytes)
	}
}

// serveV2Frontend serves the new (v2) frontend
func (s *Server) serveV2Frontend(writer http.ResponseWriter, request *http.Request) {
	v2StaticDir := getV2StaticDir()
	if v2StaticDir != "" {
		http.ServeFile(writer, request, v2StaticDir+"/index.html")
	} else {
		// Serve embedded v2 assets
		bytes, err := assetsV2.ReadFile("static-v2/index.html")
		if err != nil {
			log.Printf("Error reading v2 static assets: %v", err)
			// Fallback to v1 if v2 assets not available
			s.serveV1Frontend(writer, request)
			return
		}
		writer.Write(bytes)
	}
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

// getStaticDir returns the path to the static assets directory we use for development
// If the environment variable is not set, it returns an empty string
func getStaticDir() string {
	staticDir, ok := os.LookupEnv("STATIC_ASSETS_DIR")
	if ok {
		return filepath.Clean(staticDir)
	}

	return ""
}

// getV2StaticDir returns the path to the v2 static assets directory
func getV2StaticDir() string {
	v2StaticDir, ok := os.LookupEnv("V2_STATIC_ASSETS_DIR")
	if ok {
		return filepath.Clean(v2StaticDir)
	}

	return ""
}

// getFeatureFlag checks environment variables and configuration for the frontend version
func getFeatureFlag() bool {
	// Check environment variable first
	if v2Flag := os.Getenv("USE_V2_FRONTEND"); v2Flag != "" {
		return strings.ToLower(v2Flag) == "true" || v2Flag == "1"
	}

	// Default to v1 (false)
	return false
}

func (s *Server) Close() error {
	return s.server.Close()
}
