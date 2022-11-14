package desktopexporter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type Server struct {
	server     http.Server
	traceStore *TraceStore
}

func tracesHandler(store *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Determine how many recent traces to display
		results := request.URL.Query().Get("results")
		numTraces, err := strconv.Atoi(results)
		if err != nil {
			numTraces = len(store.traceMap)
		}

		// Get the TraceData for the requested number of traces
		traces := store.GetRecentTraces(numTraces)
		summaries := RecentSummaries{
			TraceSummaries: []TraceSummary{},
		}

		// Generate a summary for each trace
		for _, trace := range traces {
			summary, err := trace.GetTraceSummary()
			if err != nil {
				fmt.Println(err)
			} else {
				summaries.TraceSummaries = append(summaries.TraceSummaries, summary)
			}
		}

		// Marshal the TraceSummaries struct and wish it well on its journey to the kingdom of frontend.
		jsonTraceSummaries, err := json.Marshal(summaries)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonTraceSummaries)
		}
	}
}

func getTraceIDHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceStore.mut.Lock()
		defer traceStore.mut.Unlock()

		traceID := mux.Vars(request)["id"]
		jsonTrace, err := json.Marshal(traceStore.traceMap[traceID])
		if err != nil {
			fmt.Println(err)
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonTrace)
		}
	}
}

func NewServer(traceStore *TraceStore) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/traces", tracesHandler(traceStore))
	router.HandleFunc("/traces/{id}", getTraceIDHandler(traceStore))
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./desktop-exporter/static/")))

	return &Server{
		server: http.Server{
			Addr:    "localhost:8000",
			Handler: router,
		},
		traceStore: traceStore,
	}
}

func (s Server) Start() error {
	return s.server.ListenAndServe()
}

func (s Server) Close() error {
	return s.server.Close()
}
