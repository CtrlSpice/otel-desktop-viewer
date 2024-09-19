package server

import (
	"context"
	"embed"
	_ "embed"
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
)

// Maximum number of traces to keep in memory
const maxNumTraces = 10000

//go:embed static/*
var assets embed.FS

type Server struct {
	server     http.Server
	traceStore *store.Store
}

func clearDataHandler(store *store.Store) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		store.ClearTraces()
		writer.WriteHeader(http.StatusOK)
	}
}

func sampleDataHandler(store *store.Store) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := context.Background()
		for _, sd := range GenerateSampleSpanData(ctx) {
			store.Add(ctx, sd)
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

func tracesHandler(store *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Determine how many recent traces to display
		numTraces := len(store.traceMap)
		if numTraces > maxNumTraces {
			numTraces = maxNumTraces
		}

		// Get the TraceData for the requested number of traces
		traces := store.GetRecentTraces(numTraces)
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

func traceIDHandler(store *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceID := mux.Vars(request)["id"]

		traceData, err := store.GetTrace(traceID)
		if err != nil {
			fmt.Printf("error: %s\t traceID: %s\n", telemetry.ErrTraceIDNotFound, traceID)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(writer, traceData)
	}
}

func telemetryHandler(store *telemetry.Store) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Determine how many recent traces to display
		// numTraces := len(store.traceMap) // TODO: bring this back
		// if numTraces > maxNumTraces {
		// 	numTraces = maxNumTraces
		// }

		// Get the TraceData for the requested number of traces
		recent := telemetry.RecentTelemetrySummaries{
			Summaries: []telemetry.TelemetrySummary{},
		}

		// Generate a summary for each trace
		for _, td := range store.GetRecentTelemetry(maxNumTraces) {
			recent.Summaries = append(recent.Summaries, td.GetSummary())
		}

		writeJSON(writer, recent)
	}
}

func telemetryIDHandler(store *telemetry.Store) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := mux.Vars(request)["id"]

		traceData, err := store.GetTelemetry(id)
		if err != nil {
			fmt.Printf("error: %s\t telemetryID: %s\n", telemetry.ErrTraceIDNotFound, id)
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

func NewServer(traceStore *TraceStore, telemetryStore *telemetry.Store, endpoint string) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/api/traces", tracesHandler(traceStore))
	router.HandleFunc("/api/traces/{id}", traceIDHandler(traceStore))
	router.HandleFunc("/api/telemetry", telemetryHandler(telemetryStore))
	router.HandleFunc("/api/telemetry/{id}", telemetryIDHandler(telemetryStore))
	// TODO: add handlers for querying telemetry store
	router.HandleFunc("/api/sampleData", sampleDataHandler(traceStore, telemetryStore))
	router.HandleFunc("/api/clearData", clearDataHandler(traceStore, telemetryStore))
	router.HandleFunc("/traces/{id}", indexHandler)
	if os.Getenv("SERVE_FROM_FS") == "true" {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir("./desktopexporter/static/")))
	} else {
		staticContent, err := fs.Sub(assets, "static")
		if err != nil {
			log.Fatal(err)
		}
		router.PathPrefix("/").Handler(http.FileServer(http.FS(staticContent)))
	}
	return &Server{
		server: http.Server{
			Addr:    endpoint,
			Handler: router,
		},
		traceStore:     traceStore,
		telemetryStore: telemetryStore,
	}
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
