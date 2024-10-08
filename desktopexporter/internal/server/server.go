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
	"time"

	"github.com/pkg/browser"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	server http.Server
	Store  *store.Store
}

func NewServer(endpoint string) *Server {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		Store: store.NewStore(context.Background()),
	}

	serveFromFS, err := strconv.ParseBool(os.Getenv("SERVE_FROM_FS"))
	if err != nil {
		serveFromFS = false
	}

	s.server.Handler = s.Handler(serveFromFS)
	return &s
}

func (s *Server) Start() error {
	defer s.Store.Close()

	_, isCI := os.LookupEnv("CI")
	if !isCI {
		go func() {
			// Wait a bit for the server to come up to avoid a 404 as a first experience
			time.Sleep(250 * time.Millisecond)
			endpoint := s.server.Addr
			browser.OpenURL("http://" + endpoint + "/")
		}()
	}
	return s.server.ListenAndServe()
}

func (s *Server) Close() error {
	return s.server.Close()
}

func (s *Server) Handler(serveFromFS bool) http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("/api/traces", s.tracesHandler)
	router.HandleFunc("/api/traces/{id}", s.traceIDHandler)
	router.HandleFunc("/api/sampleData", s.sampleDataHandler)
	router.HandleFunc("/api/clearData", s.clearTracesHandler)
	router.HandleFunc("/traces/{id}", indexHandler)

	if serveFromFS {
		router.Handle("/", http.FileServer(http.Dir("./static/")))
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
		log.Fatalln("traceID:", traceID, "error:", err.Error())
	}
	writeJSON(writer, traceData)
}

func indexHandler(writer http.ResponseWriter, request *http.Request) {
	if os.Getenv("SERVE_FROM_FS") == "true" {
		http.ServeFile(writer, request, "./desktopexporter/internal/server/static/index.html")
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
