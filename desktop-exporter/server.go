package desktopexporter

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type Server struct {
	server     http.Server
	traceStore *TraceStore
}

func newTraceSummary(store *TraceStore, traceID string) (*TraceSummary, error) {
	spans, err := store.GetSpansByTraceID(traceID)
	if err != nil {
		return nil, err
	}

	durationMS, err := getTraceDuration(spans)
	if err != nil {
		return nil, err
	}

	return &TraceSummary{
		TraceID:    traceID,
		SpanCount:  uint32(len(spans)),
		DurationMS: durationMS,
	}, nil
}

func newTraceData(store *TraceStore, traceID string) (*TraceData, error) {
	spans, err := store.GetSpansByTraceID(traceID)
	if err != nil {
		return nil, err
	}

	return &TraceData{
		Spans: spans,
	}, nil
}

func getTracesHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// This is hardcoded until I get the value from the request
		numberOfTracesIWant := 10
		recentTraceIDs := traceStore.getRecentTraceIDs(numberOfTracesIWant)
		traceSummaries := make([]TraceSummary, 0, len(recentTraceIDs))

		for _, traceID := range recentTraceIDs {
			summary, err := newTraceSummary(traceStore, traceID)
			if err != nil {
				fmt.Println(err)
			} else {
				traceSummaries = append(traceSummaries, *summary)
			}
		}

		jsonResponse, err := json.Marshal(traceSummaries)
		if err != nil {
			panic(fmt.Errorf("error marshalling traceStore: %s\n", err))
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonResponse)
		}
	}
}

func getTraceIDHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		traceID := mux.Vars(request)["id"]
		traceData, err := newTraceData(traceStore, traceID)
		if err != nil {
			fmt.Println(err)
		} else {

		}
		jsonResponse, err := json.Marshal(traceData)
		if err != nil {
			panic(fmt.Errorf("error marshalling traceStore: %s\n", err))
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonResponse)
		}
	}
}

func NewServer(traceStore *TraceStore) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/traces", getTracesHandler(traceStore))
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
