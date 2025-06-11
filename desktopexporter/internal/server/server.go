package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	server    http.Server
	Store     *store.Store
	staticDir string
}

func NewServer(endpoint string, dbPath string) *Server {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		Store:     store.NewStore(context.Background(), dbPath),
		staticDir: getStaticDir(),
	}

	s.server.Handler = s.Handler()
	return &s
}

func (s *Server) Start() error {
	defer s.Store.Close()
	return s.server.ListenAndServe()
}

func (s *Server) Close() error {
	return s.server.Close()
}

func (s *Server) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /api/traces", s.tracesHandler)
	router.HandleFunc("GET /api/traces/{id}", s.traceIDHandler)
	router.HandleFunc("GET /api/sampleData", s.sampleDataHandler)
	router.HandleFunc("GET /api/clearTraces", s.clearTracesHandler)
	router.HandleFunc("GET /traces/{id}", s.indexHandler)
	
	// Feature flag for logs frontend route
	if os.Getenv("ENABLE_LOGS") == "true" {
		router.HandleFunc("GET /logs", s.indexHandler)
		router.HandleFunc("GET /api/logs", s.logsHandler)
		router.HandleFunc("GET /api/logs/{id}", s.logIDHandler)
		router.HandleFunc("GET /api/logs/trace/{id}", s.logsByTraceHandler)
		router.HandleFunc("GET /api/clearLogs", s.clearLogsHandler)
	}

	if s.staticDir != "" {
		router.Handle("/", http.FileServer(http.Dir(s.staticDir)))
	} else {
		staticContent, err := fs.Sub(assets, "static")
		if err != nil {
			log.Fatal(err)
		}
		router.Handle("/", http.FileServerFS(staticContent))
	}
	return router
}

func (s *Server) tracesHandler(writer http.ResponseWriter, request *http.Request) {
	summaries, err := s.Store.GetTraceSummaries(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	if err := writeJSON(writer, telemetry.TraceSummaries{
		TraceSummaries: summaries,
	}); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) clearTracesHandler(writer http.ResponseWriter, request *http.Request) {
	if err := s.Store.ClearTraces(request.Context()); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}
	writer.WriteHeader(http.StatusOK)
}

func (s *Server) clearLogsHandler(writer http.ResponseWriter, request *http.Request) {
	if err := s.Store.ClearLogs(request.Context()); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}
	writer.WriteHeader(http.StatusOK)
}

func (s *Server) logsHandler(writer http.ResponseWriter, request *http.Request) {
	logs, err := s.Store.GetLogs(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	if err := writeJSON(writer, telemetry.Logs{
		Logs: logs,
	}); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) logIDHandler(writer http.ResponseWriter, request *http.Request) {
	logID := request.PathValue("id")
	logData, err := s.Store.GetLog(request.Context(), logID)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	} else {
		if err := writeJSON(writer, logData); err != nil {
			log.Fatal(err)
		}
	}
}

func (s *Server) logsByTraceHandler(writer http.ResponseWriter, request *http.Request) {
	traceID := request.PathValue("id")
	logs, err := s.Store.GetLogsByTrace(request.Context(), traceID)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	if err := writeJSON(writer, telemetry.Logs{
		Logs: logs,
	}); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) sampleDataHandler(writer http.ResponseWriter, request *http.Request) {
	sample := telemetry.NewSampleTelemetry()
	if err := s.Store.AddSpans(request.Context(), sample.Spans); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	if err := s.Store.AddLogs(request.Context(), sample.Logs); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	writer.WriteHeader(http.StatusOK)
}

func (s *Server) traceIDHandler(writer http.ResponseWriter, request *http.Request) {
	traceID := request.PathValue("id")
	traceData, err := s.Store.GetTrace(request.Context(), traceID)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	} else {
		if err := writeJSON(writer, traceData); err != nil {
			log.Fatal(err)
		}
	}
}

func (s *Server) indexHandler(writer http.ResponseWriter, request *http.Request) {
	if s.staticDir != "" {
		http.ServeFile(writer, request, s.staticDir+"/index.html")
	} else {
		indexBytes, err := assets.ReadFile("static/index.html")
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			log.Fatalf("Error reading static assets: %s", err.Error())
		}
		writer.Write(indexBytes)
	}
}

func writeJSON(writer http.ResponseWriter, data any) error {
	writer.Header().Set("Content-Type", "application/json")

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("could not marshal JSON: %v", err)
	}

	if _, err := writer.Write(jsonBytes); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("could not write JSON response: %v", err)
	}

	return nil
}

func getStaticDir() string {
	staticDir, ok := os.LookupEnv("STATIC_ASSETS_DIR")
	if ok {
		return filepath.Clean(staticDir)
	}

	return ""
}

