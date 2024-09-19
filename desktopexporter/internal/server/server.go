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
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/browser"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// Maximum number of traces to keep in memory
const maxNumTraces = 10000

//go:embed static/*
var assets embed.FS

type Server struct {
	server http.Server
	store  *store.Store
}

func (s *Server) clearDataHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		s.store.ClearTraces()
		writer.WriteHeader(http.StatusOK)
	}
}

func (s *Server) sampleDataHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		sample := telemetry.NewSampleTelemetry()
		for _, spanData := range sample.Spans {
			s.store.Add(context.Background(), spanData)
		}
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
		// Determine how many recent traces to display
		numTraces := len(s.store.TraceMap)
		if numTraces > maxNumTraces {
			numTraces = maxNumTraces
		}

		// Get the TraceData for the requested number of traces
		traces := s.store.GetRecentTraces(numTraces)
		summaries := telemetry.RecentSummaries{
			TraceSummaries: []telemetry.TraceSummary{},
		}

		// Generate a summary for each trace
		for _, trace := range traces {
			summary := trace.GetTraceSummary()
			summaries.TraceSummaries = append(summaries.TraceSummaries, summary)
		}

		writeJSON(writer, summaries)
	}
}

func (s *Server) traceIDHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceID := mux.Vars(request)["id"]

		traceData, err := s.store.GetTrace(traceID)
		if err != nil {
			fmt.Printf("error: %s\t traceID: %s\n", telemetry.ErrTraceIDNotFound, traceID)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(writer, traceData)
	}
}

func indexHandler(writer http.ResponseWriter, request *http.Request) {
	if os.Getenv("SERVE_FROM_FS") == "true" {
		http.ServeFile(writer, request, "./desktopexporter/static/index.html")
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

	router := mux.NewRouter()
	router.HandleFunc("/api/traces", s.tracesHandler())
	router.HandleFunc("/api/traces/{id}", s.traceIDHandler())
	router.HandleFunc("/api/sampleData", s.sampleDataHandler())
	router.HandleFunc("/api/clearData", s.clearDataHandler())
	router.HandleFunc("/traces/{id}", indexHandler)
	if os.Getenv("SERVE_FROM_FS") == "true" {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	} else {
		staticContent, err := fs.Sub(assets, "static")
		if err != nil {
			log.Fatal(err)
		}
		router.PathPrefix("/").Handler(http.FileServer(http.FS(staticContent)))
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
