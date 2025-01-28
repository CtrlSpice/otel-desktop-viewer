package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	server http.Server
	Store  *store.Store
	Env    ServerEnv
}

type ServerEnv struct {
	ServeFromFS bool
	StaticDir   string
}

func NewServer(endpoint string, dbPath string) *Server {
	env := getServerEnv()
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		Store: store.NewStore(context.Background(), dbPath),
		Env:   env,
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
	router.HandleFunc("GET /api/clearData", s.clearTracesHandler)
	router.HandleFunc("GET /traces/{id}", s.indexHandler)

	if s.Env.ServeFromFS {
		router.Handle("/", http.FileServer(http.Dir(s.Env.StaticDir)))
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

	writeJSON(writer, telemetry.TraceSummaries{
		TraceSummaries: *summaries,
	})
}

func (s *Server) clearTracesHandler(writer http.ResponseWriter, request *http.Request) {
	if err := s.Store.ClearTraces(request.Context()); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}
	writer.WriteHeader(http.StatusOK)
}

func (s *Server) sampleDataHandler(writer http.ResponseWriter, request *http.Request) {
	sample := telemetry.NewSampleTelemetry()
	if err := s.Store.AddSpans(request.Context(), sample.Spans); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatal(err)
	}

	//TODO: Add sample logs and metrics
	writer.WriteHeader(http.StatusOK)
}

func (s *Server) traceIDHandler(writer http.ResponseWriter, request *http.Request) {
	traceID := request.PathValue("id")
	traceData, err := s.Store.GetTrace(request.Context(), traceID)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	} else {
		writeJSON(writer, traceData)
	}
}

func (s *Server) indexHandler(writer http.ResponseWriter, request *http.Request) {
	if s.Env.ServeFromFS {
		http.ServeFile(writer, request, s.Env.StaticDir+"/index.html")
	} else {
		indexBytes, err := assets.ReadFile("static/index.html")
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			log.Fatalf("could not read static assets: %s", err.Error())
		}
		writer.Write(indexBytes)
	}
}

func writeJSON(writer http.ResponseWriter, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Fatalf("could not marshal json: %s", err.Error())

	}

	writer.WriteHeader(http.StatusOK)
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(jsonData)
}

func getServerEnv() ServerEnv {
	serveFromFS, err := strconv.ParseBool(os.Getenv("SERVE_FROM_FS"))
	if err != nil {
		serveFromFS = false
	}

	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		serveFromFS = false
	}

	return ServerEnv{
		ServeFromFS: serveFromFS,
		StaticDir:   staticDir,
	}
}
