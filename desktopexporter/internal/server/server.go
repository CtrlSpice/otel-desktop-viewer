package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pkg/browser"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	server http.Server
	store  *store.Store
}

func (s *Server) clearDataHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		s.store.ClearTraces(request.Context())
		writer.WriteHeader(http.StatusOK)
	}
}

func (s *Server) sampleDataHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		sample := telemetry.NewSampleTelemetry()
		s.store.AddSpans(request.Context(), sample.Spans)

		//TODO: Add sample logs and metrics
		writer.WriteHeader(http.StatusOK)
	}
}

func writeJSON(writer http.ResponseWriter, data any) {
	jsonTraceData, err := json.Marshal(data)

	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusOK)
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(jsonTraceData)
}

func (s *Server) tracesHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		summaries := s.store.GetTraceSummaries(request.Context())
		writeJSON(writer, summaries)
	}
}

func (s *Server) traceIDHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceID := request.PathValue("id")

		traceData, err := s.store.GetTrace(request.Context(), traceID)
		if err != nil {
			log.Println("traceID:", traceID, "error:", err.Error())
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(writer, traceData)
	}
}

func indexHandler(writer http.ResponseWriter, request *http.Request) {
	if os.Getenv("SERVE_FROM_FS") == "true" {
		http.ServeFile(writer, request, "./desktopexporter/internal/server/static/index.html")
	} else {
		indexBytes, err := assets.ReadFile("static/index.html")
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Write(indexBytes)
	}
}

func NewServer(store *store.Store, endpoint string) *Server {
	s := Server{
		server: http.Server{
			Addr: endpoint,
		},
		store: store,
	}

	router := http.NewServeMux()
	router.HandleFunc("/api/traces", s.tracesHandler())
	router.HandleFunc("/api/traces/{id}", s.traceIDHandler())
	router.HandleFunc("/api/sampleData", s.sampleDataHandler())
	router.HandleFunc("/api/clearData", s.clearDataHandler())
	router.HandleFunc("/traces/{id}", indexHandler)
	if os.Getenv("SERVE_FROM_FS") == "true" {
		router.Handle("/", http.FileServer(http.Dir("./static/")))
	} else {
		staticContent, err := fs.Sub(assets, "static")
		if err != nil {
			log.Fatal(err)
		}
		router.Handle("/", http.FileServer(http.FS(staticContent)))
	}

	s.server.Handler = router
	return &s
}

func (s *Server) Start() error {
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
