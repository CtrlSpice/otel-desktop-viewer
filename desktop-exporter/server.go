package desktopexporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type Server struct {
	server     http.Server
	traceStore *TraceStore
}

func newTraceSummary(traceID string, spans []SpanData) (*TraceSummary, error) {
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

func getTracesHandler(store *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// This is hardcoded until I get the value from the request
		traceCount := 10
		recentTraces := store.GetRecentTraces(traceCount)
		summaries := TraceSummaries{
			Summaries: []TraceSummary{},
		}

		for traceID, spans := range recentTraces {
			summary, err := newTraceSummary(traceID, spans)
			if err != nil {
				fmt.Printf("error: %s\t traceID: %s\n", err, traceID)
			} else {
				summaries.Summaries = append(summaries.Summaries, *summary)
			}
		}

		jsonResponse, err := json.Marshal(summaries)
		if err != nil {
			fmt.Printf("error marshalling traceStore: %s\n", err)
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonResponse)
		}
	}
}

func getTraceIDHandler(store *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {

		traceID := mux.Vars(request)["id"]
		traceData, err := newTraceData(store, traceID)
		if err != nil {
			fmt.Println(err)
		} else {

		}
		jsonResponse, err := json.Marshal(traceData)
		if err != nil {
			fmt.Printf("error marshalling traceStore: %s\n", err)
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonResponse)
		}
	}
}

func NewServer(store *TraceStore) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/traces", getTracesHandler(store))
	router.HandleFunc("/traces/{id}", getTraceIDHandler(store))
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./desktop-exporter/static/")))

	return &Server{
		server: http.Server{
			Addr:    "localhost:8000",
			Handler: router,
		},
		traceStore: store,
	}
}

func (s Server) Start() error {
	return s.server.ListenAndServe()
}

func (s Server) Close() error {
	return s.server.Close()
}

func getTraceDuration(spans []SpanData) (int64, error) {
	if len(spans) < 1 {
		return 0, errors.New("can't calculate trace duration - spans slice is empty")
	}

	// Determine the total duration of the trace
	traceStartTime := spans[0].StartTime
	traceEndTime := spans[0].EndTime
	for i := 1; i < len(spans); i++ {
		if spans[i].StartTime.Before(traceStartTime) {
			traceStartTime = spans[i].StartTime
		}

		if spans[i].EndTime.After(traceEndTime) {
			traceEndTime = spans[i].EndTime
		}
	}
	return traceEndTime.Sub(traceStartTime).Milliseconds(), nil
}
