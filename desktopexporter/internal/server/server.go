package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/browser"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

//go:embed static/*
var assets embed.FS

type EnvConfig struct {
	IsDev     bool
	IsCI      bool
	StaticDir string
}

type Server struct {
	server http.Server
	Store  *store.Store
	Env    EnvConfig
}

func NewServer(endpoint string, dbPath string) *Server {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		Store: store.NewStore(context.Background(), dbPath),
		Env:   GetEnvConfig(),
	}
	s.server.Handler = s.Handler()
	return &s
}

func (s *Server) Start() error {
	defer s.Store.Close()

	if !s.Env.IsCI {
		go func() {
			// Wait a bit for the server to come up to avoid a 404 as a first experience
			time.Sleep(250 * time.Millisecond)
			endpoint := s.server.Addr
			browser.OpenURL("http://" + endpoint)
		}()
	}
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

	if s.Env.IsDev {
		log.Println(s.Env.StaticDir)
		// Serve front-end from the filesystem
		router.Handle("/", http.FileServer(http.Dir("/Users/T998182/Workspace/otel-desktop-viewer/desktopexporter/internal/server/static/")))
	} else {
		// Serve front end from embedded static content
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
	if s.Env.IsDev {
		log.Println("IDX HANDLER RAN " + s.Env.StaticDir)
		//http.ServeFile(writer, request, s.Env.StaticDir)
		http.ServeFile(writer, request, "/Users/T998182/Workspace/otel-desktop-viewer/desktopexporter/internal/server/static/index.html")
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

func GetEnvConfig() EnvConfig {
	isDev := isSet("DEV")
	isCI := isSet("CI")
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalln("could not get current directory to set env config")
	}

	relStaticDir, sdSet := os.LookupEnv("STATIC_DIR")
	if isDev && !sdSet {
		log.Fatalln("the STATIC_DIR environment variable must be set when working in DEV")
	}

	sd := wd + filepath.Clean(relStaticDir)

	return EnvConfig{
		IsDev:     isDev,
		IsCI:      isCI,
		StaticDir: sd,
	}
}

func isSet(key string) bool {
	str, ok := os.LookupEnv(key)
	if !ok {
		return false
	}

	val, err := strconv.ParseBool(str)
	if err != nil {
		return false
	}

	return val
}
